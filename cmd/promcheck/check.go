package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	promql "github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"

	"github.com/cbrgm/promcheck/internal/checker"
	"github.com/cbrgm/promcheck/internal/metrics"
	"github.com/cbrgm/promcheck/internal/report"
)

// ErrNoRuleGroups is returned when a check run finds no rule groups to probe.
var ErrNoRuleGroups = errors.New("no rule groups to check")

// ErrStrictFindings is returned by runCheck when --strict is set and one or
// more selectors had no results. In exporter mode this sentinel is swallowed
// (runCheck returns nil instead) so a dead rule can't kill the exporter loop.
var ErrStrictFindings = errors.New("strict: selectors without results found")

type Reporter interface {
	Dump() error
	AddSection(file, group, name, expression string, failed, success []string)
	AddTotalCheckedGroups(count int)
}

type Checker interface {
	CheckRuleGroup(ctx context.Context, group checker.RuleGroup) ([]checker.CheckResult, error)
	IsIgnoredGroup(name string) bool
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
	optCheckMatch                   []string
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
	rulesChecker, err := checker.NewPrometheusRulesChecker(
		checker.PrometheusRulesCheckerConfig{
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

	// Color only when writing to a real terminal, NO_COLOR isn't set (see
	// no-color.org), and the user didn't pass --output.no-color.
	useColor := report.IsTTY(os.Stdout) && os.Getenv("NO_COLOR") == "" && !config.OutputNoColor
	reportOptions := []report.BuilderOption{
		report.WithFormat(config.OutputFormat),
		report.WithMetrics(promMetrics),
		report.WithColor(useColor),
	}
	if config.OutputOnlyFailing {
		reportOptions = append(reportOptions, report.WithOnlyFailing())
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
		optCheckMatch:                   config.CheckMatch,
		optStrictMode:                   config.StrictMode,

		// internal
		check:        rulesChecker,
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

	// One-shot runs otherwise have no way to react to Ctrl-C: without this,
	// checkRules gets an uncancellable context and a SIGINT during a slow
	// probe just kills the process outright instead of unwinding gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return app.checkRules(ctx)
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
	load(ctx context.Context) ([]checker.RuleGroup, error)
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

	// Filter out ignored groups up front so they are neither probed, nor
	// counted, nor rendered.
	groups = slices.DeleteFunc(groups, func(g checker.RuleGroup) bool {
		return app.check.IsIgnoredGroup(g.Name)
	})

	var (
		mu           sync.Mutex
		checkResults []checker.CheckResult
	)
	eg, ctx := errgroup.WithContext(ctx)

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
		if app.optExporterModeEnabled {
			// The exporter is a long-running process; a dead rule must not
			// kill it, so the sentinel is swallowed here.
			return nil
		}
		return ErrStrictFindings
	}
	return app.report.Dump()
}

// fileSource loads rule groups from rule files matched by a glob pattern.
type fileSource struct {
	app         *promcheckApp
	filesRegexp string
}

func (s fileSource) name() string { return "file" }

func (s fileSource) load(_ context.Context) ([]checker.RuleGroup, error) {
	matches, err := filepath.Glob(s.filesRegexp)
	if err != nil {
		s.app.logger.Error("failed to parse rule group file paths", "err", err)
		return nil, err
	}

	groups := []checker.RuleGroup{}
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

func processFile(p promql.Parser, logger *slog.Logger, file string) ([]checker.RuleGroup, error) {
	ruleGroups, errs := rulefmt.ParseFile(file, false, model.UTF8Validation, p, logger)
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s: %w", file, errors.Join(errs...))
	}

	converted := make([]checker.RuleGroup, 0, len(ruleGroups.Groups))
	for _, group := range ruleGroups.Groups {
		converted = append(converted, rulefmtToPromcheck(file, group))
	}
	return converted, nil
}

func rulefmtToPromcheck(fileName string, group rulefmt.RuleGroup) checker.RuleGroup {
	out := checker.RuleGroup{Name: group.Name, File: fileName, Rules: make([]checker.Rule, 0, len(group.Rules))}
	if group.QueryOffset != nil {
		// model.Duration is a typedef of time.Duration.
		out.QueryOffset = time.Duration(*group.QueryOffset)
	}
	for _, rule := range group.Rules {
		name := cmp.Or(rule.Record, rule.Alert)
		out.Rules = append(out.Rules, checker.Rule{Name: name, Expression: rule.Expr})
	}
	return out
}

// instanceSource loads rule groups from a live Prometheus instance.
type instanceSource struct {
	app      *promcheckApp
	api      prometheusv1.API
	matchers []string
}

func (s instanceSource) name() string { return "instance" }

func (s instanceSource) load(ctx context.Context) ([]checker.RuleGroup, error) {
	apiResponse, err := s.api.Rules(ctx, s.matchers)
	if err != nil {
		s.app.logger.Error("failed to receive rules from prometheus instance", "err", err)
		return nil, err
	}

	groups := make([]checker.RuleGroup, 0, len(apiResponse.Groups))
	for _, group := range apiResponse.Groups {
		groups = append(groups, prometheusv1ToPromcheck(group))
	}
	return groups, nil
}

func (app *promcheckApp) checkRulesFromPrometheusInstance(ctx context.Context) error {
	client, err := api.NewClient(api.Config{
		Address:      app.optPrometheusURL,
		RoundTripper: app.roundTripper,
	})
	if err != nil {
		app.logger.Error("failed to create Prometheus client", "err", err)
		return err
	}
	promAPI := prometheusv1.NewAPI(client)
	return app.runCheck(ctx, instanceSource{app: app, api: promAPI, matchers: app.optCheckMatch})
}

func prometheusv1ToPromcheck(group prometheusv1.RuleGroup) checker.RuleGroup {
	convertedRuleGroup := checker.RuleGroup{
		Name:  group.Name,
		File:  group.File,
		Rules: []checker.Rule{},
	}
	for _, rule := range group.Rules {
		switch v := rule.(type) {
		case prometheusv1.RecordingRule:
			convertedRuleGroup.Rules = append(convertedRuleGroup.Rules, checker.Rule{
				Name:       v.Name,
				Expression: v.Query,
			})
		case prometheusv1.AlertingRule:
			convertedRuleGroup.Rules = append(convertedRuleGroup.Rules, checker.Rule{
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

func (s inlineSource) load(_ context.Context) ([]checker.RuleGroup, error) {
	group := checker.RuleGroup{
		Name:  "[inline]",
		File:  "[manual]",
		Rules: []checker.Rule{},
	}
	for i, query := range s.expressions {
		group.Rules = append(group.Rules, checker.Rule{
			Name:       fmt.Sprintf("query-%d", i),
			Expression: query,
		})
	}
	return []checker.RuleGroup{group}, nil
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
