package main

import (
	"fmt"
	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"path/filepath"
	"time"

	"github.com/cbrgm/promcheck/promcheck"
	"github.com/cbrgm/promcheck/promcheck/report"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/prometheus/model/rulefmt"
)

func checkRulesFromRuleFiles(config *config, logger log.Logger) error {
	var (
		delay            = time.Duration(config.CheckDelay) * time.Second
		prometheusUrl    = config.PrometheusUrl
		ignoredSelectors = config.CheckIgnoredSelectorsRegexp
		ignoredGroups    = config.CheckIgnoredGroupsRegexp
		filePaths        = config.CheckFiles
		outputFormat     = config.OutputFormat
		outputNoColor    = config.OutputNoColor
	)

	client, err := api.NewClient(api.Config{Address: prometheusUrl})
	if err != nil {
		level.Error(logger).Log("msg", "failed to create Prometheus client", "err", err)
		return err
	}

	checker := promcheck.NewPrometheusRulesChecker(
		promcheck.PrometheusRulesCheckerConfig{
			ProbeDelay:             delay,
			PrometheusUrl:          prometheusUrl,
			IgnoredSelectorsRegexp: ignoredSelectors,
			IgnoredGroupsRegexp:    ignoredGroups,
		},
		prometheusv1.NewAPI(client),
	)

	builder := report.NewBuilder(
		outputFormat,
		outputNoColor,
	)

	matches, err := filepath.Glob(fmt.Sprintf("%s", filePaths))
	if err != nil {
		level.Error(logger).Log("msg", "failed to parse rule group file paths", "err", err)
		return err
	}

	filesToCheck := []rulesFile{}
	for _, file := range matches {
		fileToCheck, err := processFile(file)
		if err != nil {
			level.Error(logger).Log("msg", "failed to parse rule group files", "err", err)
			return err
		}
		filesToCheck = append(filesToCheck, fileToCheck)
	}

	res := []promcheck.CheckResult{}
	for _, file := range filesToCheck {
		checked, err := checker.CheckRuleGroups(file.File, file.groups)
		if err != nil {
			level.Error(logger).Log("msg", "failed to check rule groups", "file", file, "err", err)
			return err
		}

		res = append(res, checked...)

		// count checked rules
		for _, group := range file.groups {
			builder.AddTotalCheckedGroups(1)
			builder.AddTotalCheckedRules(len(group.Rules))
		}
	}
	for _, cr := range res {
		builder.AddSection(
			cr.File,
			cr.Group,
			cr.Name,
			cr.Expression,
			cr.Results,
			cr.NoResults,
		)
	}
	return builder.Dump()
}

type rulesFile struct {
	File   string
	groups []rulefmt.RuleGroup
}

func processFile(file string) (rulesFile, error) {
	ruleGroups, errs := rulefmt.ParseFile(file)
	if len(errs) > 0 {
		return rulesFile{
			File:   file,
			groups: []rulefmt.RuleGroup{},
		}, fmt.Errorf("%s", errs)
	}

	return rulesFile{
		File:   file,
		groups: ruleGroups.Groups,
	}, nil
}
