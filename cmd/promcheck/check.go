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
	optConcurrency                  int

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
		optConcurrency:                  config.CheckConcurrency,

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
	return app.checkRules()
}

func (app *promcheckApp) checkRules() error {
	if len(app.optInlineExpressions) > 0 {
		return app.checkRulesFromInlineQueries()
	}
	if app.optFilesRegexp != "" {
		return app.checkRulesFromRuleFiles()
	}
	return app.checkRulesFromPrometheusInstance()
}

func (app *promcheckApp) checkRulesFromRuleFiles() error {
	matches, err := filepath.Glob(app.optFilesRegexp)
	if err != nil {
		app.logger.Error("failed to parse rule group file paths", "err", err)
		return err
	}

	ruleGroupsToCheck := []promcheck.RuleGroup{}
	for _, file := range matches {
		ruleGroups, err := processFile(app.parser, app.logger, file)
		if err != nil {
			app.logger.Error("failed to parse rule group files", "err", err)
			return err
		}
		ruleGroupsToCheck = append(ruleGroupsToCheck, ruleGroups...)
	}

	if len(ruleGroupsToCheck) == 0 {
		app.logger.Error("no rule groups to check. Please check for --check.file flag spelling mistakes")
		return ErrNoRuleGroups
	}

	var eg errgroup.Group
	checkResults := []promcheck.CheckResult{}
	resultChan := make(chan promcheck.CheckResult, len(ruleGroupsToCheck))

	for _, group := range ruleGroupsToCheck {
		group := group // https://golang.org/doc/faq#closures_and_goroutines
		eg.Go(func() error {
			checked, err := app.check.CheckRuleGroup(context.TODO(), group)
			if err != nil {
				app.logger.Error("failed to check rule groups", "file", group.File, "err", err)
				return err
			}
			for _, res := range checked {
				resultChan <- res
			}
			app.report.AddTotalCheckedGroups(1)
			return nil
		})
	}

	go func() {
		if err := eg.Wait(); err != nil {
			app.logger.Error("failed to check rule groups", "err", err)
			close(resultChan)
			return
		}
		close(resultChan)
	}()

	for res := range resultChan {
		checkResults = append(checkResults, res)
	}

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
		err := app.report.Dump()
		if err != nil {
			app.logger.Error("failed to print report", "err", err)
		}
		os.Exit(1)
	}
	return app.report.Dump()
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

func (app *promcheckApp) checkRulesFromPrometheusInstance() error {
	client, err := api.NewClient(api.Config{
		Address:      app.optPrometheusURL,
		RoundTripper: app.roundTripper,
	})
	if err != nil {
		app.logger.Error("failed to create Prometheus client", "err", err)
		return err
	}
	promAPI := prometheusv1.NewAPI(client)
	apiResponse, err := promAPI.Rules(context.TODO(), nil) // TODO: Can we somehow only load the ones we're interested in if filtered?
	if err != nil {
		app.logger.Error("failed to receive rules from prometheus instance", "err", err)
		return err
	}

	ruleGroupsToCheck := make([]promcheck.RuleGroup, 0, len(apiResponse.Groups))
	for _, group := range apiResponse.Groups {
		ruleGroupsToCheck = append(ruleGroupsToCheck, prometheusv1ToPromcheck(group))
	}

	if len(ruleGroupsToCheck) == 0 {
		app.logger.Error("no rule groups to check. Please check whether the Prometheus instance contains any rules.")
		return ErrNoRuleGroups
	}

	var eg errgroup.Group
	checkResults := []promcheck.CheckResult{}
	resultChan := make(chan promcheck.CheckResult, len(ruleGroupsToCheck))

	for _, group := range ruleGroupsToCheck {
		group := group // https://golang.org/doc/faq#closures_and_goroutines
		eg.Go(func() error {
			checked, err := app.check.CheckRuleGroup(context.TODO(), group)
			if err != nil {
				app.logger.Error("failed to check rule groups", "file", group.File, "err", err)
				return err
			}
			for _, res := range checked {
				resultChan <- res
			}
			app.report.AddTotalCheckedGroups(1)
			return nil
		})
	}

	go func() {
		if err := eg.Wait(); err != nil {
			app.logger.Error("failed to check rule groups", "err", err)
			close(resultChan)
			return
		}
		close(resultChan)
	}()

	for res := range resultChan {
		checkResults = append(checkResults, res)
	}
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
		err := app.report.Dump()
		if err != nil {
			app.logger.Error("failed to print report", "err", err)
		}
		os.Exit(1)
	}
	return app.report.Dump()
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

func (app *promcheckApp) checkRulesFromInlineQueries() error {
	group := promcheck.RuleGroup{
		Name:  "[inline]",
		File:  "[manual]",
		Rules: []promcheck.Rule{},
	}
	for i, query := range app.optInlineExpressions {
		group.Rules = append(group.Rules, promcheck.Rule{
			Name:       fmt.Sprintf("query-%d", i),
			Expression: query,
		})
	}

	checkResults := []promcheck.CheckResult{}
	checked, err := app.check.CheckRuleGroup(context.TODO(), group)
	if err != nil {
		app.logger.Error("failed to check rule groups", "file", group.File, "err", err)
		return err
	}

	checkResults = append(checkResults, checked...)
	app.report.AddTotalCheckedGroups(1)

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
		err := app.report.Dump()
		if err != nil {
			app.logger.Error("failed to print report", "err", err)
		}
		os.Exit(1)
	}
	return app.report.Dump()
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
