package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/prometheus/model/rulefmt"

	"github.com/cbrgm/promcheck/promcheck"
	"github.com/cbrgm/promcheck/promcheck/metrics"
	"github.com/cbrgm/promcheck/promcheck/report"
)

type Reporter interface {
	Dump() error
	AddSection(file, group, name, expression string, failed, success []string)
	AddTotalCheckedGroups(count int)
	AddTotalCheckedRules(count int)
}

type Checker interface {
	CheckRuleGroup(group promcheck.RuleGroup) ([]promcheck.CheckResult, error)
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

	check   Checker
	report  Reporter
	logger  log.Logger
	metrics metrics.Metrics
}

func newPromcheck(config *config, logger log.Logger) (*promcheckApp, error) {
	// write prometheus metrics when exporter mode is enabled
	if config.ExporterModeEnabled {
		config.OutputFormat = report.PrometheusFormat
	}

	client, err := api.NewClient(api.Config{Address: config.PrometheusURL})
	if err != nil {
		level.Error(logger).Log("msg", "failed to create Prometheus client", "err", err)
		return nil, err
	}

	promAPI := prometheusv1.NewAPI(client)
	checker := promcheck.NewPrometheusRulesChecker(
		promcheck.PrometheusRulesCheckerConfig{
			ProbeDelay:             time.Duration(config.CheckDelay) * time.Second,
			PrometheusURL:          config.PrometheusURL,
			IgnoredSelectorsRegexp: config.CheckIgnoredSelectorsRegexp,
			IgnoredGroupsRegexp:    config.CheckIgnoredGroupsRegexp,
		},
		promAPI,
	)

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

		// internal
		check:   checker,
		report:  reporter,
		logger:  logger,
		metrics: promMetrics,
	}, nil
}

func (app *promcheckApp) run() error {
	if app.optExporterModeEnabled {
		return app.runPromcheckExporter()
	}
	return app.checkRules()
}

func (app *promcheckApp) checkRules() error {
	if app.optFilesRegexp != "" {
		return app.checkRulesFromRuleFiles()
	}
	return app.checkRulesFromPrometheusInstance()
}

func (app *promcheckApp) checkRulesFromRuleFiles() error {
	matches, err := filepath.Glob(app.optFilesRegexp)
	if err != nil {
		level.Error(app.logger).Log("msg", "failed to parse rule group file paths", "err", err)
		return err
	}

	ruleGroupsToCheck := []promcheck.RuleGroup{}
	for _, file := range matches {
		ruleGroups, err := processFile(file)
		if err != nil {
			level.Error(app.logger).Log("msg", "failed to parse rule group files", "err", err)
			return err
		}
		ruleGroupsToCheck = append(ruleGroupsToCheck, ruleGroups...)
	}

	if len(ruleGroupsToCheck) == 0 {
		level.Error(app.logger).Log("msg", "no rule groups to check. Please check for --check.file flag spelling mistakes")
		return err
	}

	checkResults := []promcheck.CheckResult{}
	for _, group := range ruleGroupsToCheck {
		checked, err := app.check.CheckRuleGroup(group)
		if err != nil {
			level.Error(app.logger).Log("msg", "failed to check rule groups", "file", group.File, "err", err)
			return err
		}
		checkResults = append(checkResults, checked...)
		app.report.AddTotalCheckedGroups(1)
		app.report.AddTotalCheckedRules(len(group.Rules))
	}
	for _, cr := range checkResults {
		app.report.AddSection(
			cr.File,
			cr.Group,
			cr.Name,
			cr.Expression,
			cr.Results,
			cr.NoResults,
		)
	}
	return app.report.Dump()
}

func processFile(file string) ([]promcheck.RuleGroup, error) {
	ruleGroups, errs := rulefmt.ParseFile(file)
	if len(errs) > 0 {
		return []promcheck.RuleGroup{}, fmt.Errorf("%s", errs)
	}

	converted := []promcheck.RuleGroup{}
	for _, group := range ruleGroups.Groups {
		converted = append(converted, rulefmtToPromcheck(file, group))
	}
	return converted, nil
}

func rulefmtToPromcheck(fileName string, group rulefmt.RuleGroup) promcheck.RuleGroup {
	convertedRuleGroup := promcheck.RuleGroup{
		Name:  group.Name,
		File:  fileName,
		Rules: []promcheck.Rule{},
	}
	for _, rule := range group.Rules {
		var name string
		if rule.Record.Value == "" {
			name = rule.Alert.Value
		}
		if rule.Alert.Value == "" {
			name = rule.Record.Value
		}
		convertedRuleGroup.Rules = append(convertedRuleGroup.Rules, promcheck.Rule{
			Name:       name,
			Expression: rule.Expr.Value,
		})
	}
	return convertedRuleGroup
}

func (app *promcheckApp) checkRulesFromPrometheusInstance() error {
	client, err := api.NewClient(api.Config{Address: app.optPrometheusURL})
	if err != nil {
		level.Error(app.logger).Log("msg", "failed to create Prometheus client", "err", err)
		return err
	}
	promAPI := prometheusv1.NewAPI(client)
	apiResponse, err := promAPI.Rules(context.TODO()) // TODO: Can we somehow only load the ones we're interested in if filtered?
	if err != nil {
		level.Error(app.logger).Log("msg", "failed to receive rules from prometheus instance", "err", err)
		return err
	}

	ruleGroupsToCheck := make([]promcheck.RuleGroup, 0, len(apiResponse.Groups))
	for _, group := range apiResponse.Groups {
		ruleGroupsToCheck = append(ruleGroupsToCheck, prometheusv1ToPromcheck(group))
	}

	if len(ruleGroupsToCheck) == 0 {
		level.Error(app.logger).Log("msg", "no rule groups to check. Please check whether the Prometheus instance contains any rules.")
		return err
	}

	checkResults := []promcheck.CheckResult{}
	for _, group := range ruleGroupsToCheck {
		checked, err := app.check.CheckRuleGroup(group)
		if err != nil {
			level.Error(app.logger).Log("msg", "failed to check rule groups", "file", group.File, "err", err)
			return err
		}
		checkResults = append(checkResults, checked...)
		app.report.AddTotalCheckedGroups(1)
		app.report.AddTotalCheckedRules(len(group.Rules))
	}
	for _, cr := range checkResults {
		app.report.AddSection(
			cr.File,
			cr.Group,
			cr.Name,
			cr.Expression,
			cr.Results,
			cr.NoResults,
		)
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
