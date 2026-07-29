package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	promql "github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"

	"github.com/cbrgm/promcheck/promcheck"
	"github.com/cbrgm/promcheck/promcheck/metrics"
	"github.com/cbrgm/promcheck/promcheck/report"
)

// ErrNoRuleGroups is returned when a check run finds no rule groups to probe.
var ErrNoRuleGroups = errors.New("no rule groups to check")

type Reporter interface {
	Dump() error
	AddSection(file, group, name, expression string, failed, success []string)
	AddTotalCheckedGroups(count int)
}

type Checker interface {
	CheckRuleGroup(ctx context.Context, group promcheck.RuleGroup) ([]promcheck.CheckResult, error)
}

type promcheckApp struct {
	optExporterHTTPAddr             string
	optExporterInterval             time.Duration
	optExporterEnableProfiling      bool
	optExporterEnableRuntimeMetrics bool
	optExporterMetricsPrefix        string
	optExporterModeEnabled          bool
	optPrometheusURL                string
	optFilesRegexp                  string
	optInlineExpressions            []string
	optStrictMode                   bool

	check        Checker
	report       Reporter
	logger       *slog.Logger
	metrics      metrics.Metrics
	roundTripper http.RoundTripper
	parser       promql.Parser
}

func newPromcheck(config *config, logger *slog.Logger) (*promcheckApp, error) {
	// write prometheus metrics when exporter mode is enabled
	if config.ExporterModeEnabled {
		config.OutputFormat = report.PrometheusFormat
	}

	roundTripper := api.DefaultRoundTripper
	if config.PrometheusBasicAuthUsername != "" && config.PrometheusBasicAuthPassword != "" {
		roundTripper = NewBasicAuthRoundTripper(
			config.PrometheusBasicAuthUsername,
			config.PrometheusBasicAuthPassword,
			api.DefaultRoundTripper,
		)
	}

	client, err := api.NewClient(api.Config{
		Address:      config.PrometheusURL,
		RoundTripper: roundTripper,
	})
	if err != nil {
		logger.Error("failed to create Prometheus client", "err", err)
		return nil, err
	}

	promAPI := prometheusv1.NewAPI(client)
	checker, err := promcheck.NewPrometheusRulesChecker(
		promcheck.PrometheusRulesCheckerConfig{
			PrometheusURL:          config.PrometheusURL,
			IgnoredSelectorsRegexp: config.CheckIgnoredSelectorsRegexp,
			IgnoredGroupsRegexp:    config.CheckIgnoredGroupsRegexp,
			MaxConcurrency:         config.CheckConcurrency,
		},
		promAPI,
	)
	if err != nil {
		logger.Error("failed to create rules checker", "err", err)
		return nil, err
	}

	promMetrics := metrics.NewPrometheus(metrics.Options{
		Prefix:               config.ExporterMetricsPrefix,
		EnableProfile:        config.ExporterEnableProfiling,
		EnableRuntimeMetrics: config.ExporterEnableRuntimeMetrics,
		PrometheusRegistry:   nil,
	})

	reportOptions := []report.BuilderOption{
		report.WithFormat(config.OutputFormat),
		report.WithMetrics(promMetrics),
	}
	if config.OutputNoColor {
		reportOptions = append(reportOptions, report.WithoutColor())
	}
	reporter := report.NewBuilder(reportOptions...)

	return &promcheckApp{
		// options
		optExporterHTTPAddr:             config.ExporterHTTPAddr,
		optExporterInterval:             time.Duration(config.ExporterInterval) * time.Second,
		optExporterEnableProfiling:      config.ExporterEnableProfiling,
		optExporterEnableRuntimeMetrics: config.ExporterEnableRuntimeMetrics,
		optExporterMetricsPrefix:        config.ExporterMetricsPrefix,
		optExporterModeEnabled:          config.ExporterModeEnabled,
		optPrometheusURL:                config.PrometheusURL,
		optFilesRegexp:                  config.CheckFiles,
		optInlineExpressions:            config.CheckExpressions,
		optStrictMode:                   config.StrictMode,

		// internal
		check:        checker,
		report:       reporter,
		logger:       logger,
		metrics:      promMetrics,
		roundTripper: roundTripper,
		parser:       promql.NewParser(promql.Options{}),
	}, nil
}

func (app *promcheckApp) run() error {
	if app.optExporterModeEnabled {
		return app.runPromcheckExporter()
	}
	return app.checkRules(context.Background())
}

func (app *promcheckApp) checkRules(ctx context.Context) error {
	if len(app.optInlineExpressions) > 0 {
		return app.checkRulesFromInlineQueries(ctx)
	}
	if app.optFilesRegexp != "" {
		return app.checkRulesFromRuleFiles(ctx)
	}
	return app.checkRulesFromPrometheusInstance(ctx)
}

// ruleSource yields the rule groups to check.
type ruleSource interface {
	load(ctx context.Context) ([]promcheck.RuleGroup, error)
	name() string
}

// runCheck loads rule groups from src, probes them concurrently, aggregates the
// results into the report, and handles strict mode. It is shared by all check modes.
func (app *promcheckApp) runCheck(ctx context.Context, src ruleSource) error {
	groups, err := src.load(ctx)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		app.logger.Error("no rule groups to check", "source", src.name())
		return ErrNoRuleGroups
	}

	var (
		mu           sync.Mutex
		checkResults []promcheck.CheckResult
		eg           errgroup.Group
	)

	// The outer fan-out over groups is unbounded; total probe concurrency is
	// bounded inside the checker (see PrometheusRulesCheckerConfig.MaxConcurrency).
	for _, group := range groups {
		eg.Go(func() error {
			checked, err := app.check.CheckRuleGroup(ctx, group)
			if err != nil {
				app.logger.Error("failed to check rule group", "file", group.File, "group", group.Name, "err", err)
				return err
			}
			mu.Lock()
			checkResults = append(checkResults, checked...)
			mu.Unlock()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	app.report.AddTotalCheckedGroups(len(groups))

	hasExpressionsWithoutResult := false
	for _, cr := range checkResults {
		app.report.AddSection(
			cr.File,
			cr.Group,
			cr.Name,
			cr.Expression,
			cr.NoResults,
			cr.Results,
		)
		if len(cr.NoResults) > 0 {
			hasExpressionsWithoutResult = true
		}
	}
	if hasExpressionsWithoutResult && app.optStrictMode {
		if err := app.report.Dump(); err != nil {
			app.logger.Error("failed to print report", "err", err)
		}
		os.Exit(1)
	}
	return app.report.Dump()
}

// fileSource loads rule groups from rule files matched by a glob pattern.
type fileSource struct {
	app         *promcheckApp
	filesRegexp string
}

func (s fileSource) name() string { return "file" }

func (s fileSource) load(_ context.Context) ([]promcheck.RuleGroup, error) {
	matches, err := filepath.Glob(s.filesRegexp)
	if err != nil {
		s.app.logger.Error("failed to parse rule group file paths", "err", err)
		return nil, err
	}

	groups := []promcheck.RuleGroup{}
	for _, file := range matches {
		parsed, err := processFile(s.app.parser, s.app.logger, file)
		if err != nil {
			s.app.logger.Error("failed to parse rule group files", "err", err)
			return nil, err
		}
		groups = append(groups, parsed...)
	}
	return groups, nil
}

func (app *promcheckApp) checkRulesFromRuleFiles(ctx context.Context) error {
	return app.runCheck(ctx, fileSource{app: app, filesRegexp: app.optFilesRegexp})
}

func processFile(p promql.Parser, logger *slog.Logger, file string) ([]promcheck.RuleGroup, error) {
	ruleGroups, errs := rulefmt.ParseFile(file, false, model.UTF8Validation, p, logger)
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s: %w", file, errors.Join(errs...))
	}

	converted := make([]promcheck.RuleGroup, 0, len(ruleGroups.Groups))
	for _, group := range ruleGroups.Groups {
		converted = append(converted, rulefmtToPromcheck(file, group))
	}
	return converted, nil
}

func rulefmtToPromcheck(fileName string, group rulefmt.RuleGroup) promcheck.RuleGroup {
	out := promcheck.RuleGroup{Name: group.Name, File: fileName, Rules: make([]promcheck.Rule, 0, len(group.Rules))}
	for _, rule := range group.Rules {
		name := cmp.Or(rule.Record, rule.Alert)
		out.Rules = append(out.Rules, promcheck.Rule{Name: name, Expression: rule.Expr})
	}
	return out
}

// instanceSource loads rule groups from a live Prometheus instance.
type instanceSource struct {
	app *promcheckApp
}

func (s instanceSource) name() string { return "instance" }

func (s instanceSource) load(ctx context.Context) ([]promcheck.RuleGroup, error) {
	client, err := api.NewClient(api.Config{
		Address:      s.app.optPrometheusURL,
		RoundTripper: s.app.roundTripper,
	})
	if err != nil {
		s.app.logger.Error("failed to create Prometheus client", "err", err)
		return nil, err
	}
	promAPI := prometheusv1.NewAPI(client)
	// matchers stay nil for now; server-side matcher filtering arrives in Task 4.1.
	apiResponse, err := promAPI.Rules(ctx, nil)
	if err != nil {
		s.app.logger.Error("failed to receive rules from prometheus instance", "err", err)
		return nil, err
	}

	groups := make([]promcheck.RuleGroup, 0, len(apiResponse.Groups))
	for _, group := range apiResponse.Groups {
		groups = append(groups, prometheusv1ToPromcheck(group))
	}
	return groups, nil
}

func (app *promcheckApp) checkRulesFromPrometheusInstance(ctx context.Context) error {
	return app.runCheck(ctx, instanceSource{app: app})
}

func prometheusv1ToPromcheck(group prometheusv1.RuleGroup) promcheck.RuleGroup {
	convertedRuleGroup := promcheck.RuleGroup{
		Name:  group.Name,
		File:  group.File,
		Rules: []promcheck.Rule{},
	}
	for _, rule := range group.Rules {
		switch v := rule.(type) {
		case prometheusv1.RecordingRule:
			convertedRuleGroup.Rules = append(convertedRuleGroup.Rules, promcheck.Rule{
				Name:       v.Name,
				Expression: v.Query,
			})
		case prometheusv1.AlertingRule:
			convertedRuleGroup.Rules = append(convertedRuleGroup.Rules, promcheck.Rule{
				Name:       v.Name,
				Expression: v.Query,
			})
		}
	}
	return convertedRuleGroup
}

// inlineSource builds a single synthetic rule group from inline PromQL queries.
type inlineSource struct {
	expressions []string
}

func (s inlineSource) name() string { return "inline" }

func (s inlineSource) load(_ context.Context) ([]promcheck.RuleGroup, error) {
	group := promcheck.RuleGroup{
		Name:  "[inline]",
		File:  "[manual]",
		Rules: []promcheck.Rule{},
	}
	for i, query := range s.expressions {
		group.Rules = append(group.Rules, promcheck.Rule{
			Name:       fmt.Sprintf("query-%d", i),
			Expression: query,
		})
	}
	return []promcheck.RuleGroup{group}, nil
}

func (app *promcheckApp) checkRulesFromInlineQueries(ctx context.Context) error {
	return app.runCheck(ctx, inlineSource{expressions: app.optInlineExpressions})
}

type basicAuthRoundTripper struct {
	username string
	password string
	rt       http.RoundTripper
}

// NewBasicAuthRoundTripper will apply a BASIC auth authorization header to a
// request unless it has already been set.
func NewBasicAuthRoundTripper(username, password string, rt http.RoundTripper) http.RoundTripper {
	return &basicAuthRoundTripper{username, password, rt}
}

func (rt *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.rt.RoundTrip(req)
	}
	req.SetBasicAuth(rt.username, rt.password)
	return rt.rt.RoundTrip(req)
}
