package promcheck

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sync"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/prometheus/prometheus/model/labels"
	promql "github.com/prometheus/prometheus/promql/parser"

	"golang.org/x/sync/errgroup"
)

// PrometheusRulesCheckerConfig represents PrometheusRulesChecker configuration.
type PrometheusRulesCheckerConfig struct {
	// PrometheusURL represents the Prometheus instance url
	PrometheusURL string

	// IgnoredSelectorsRegexp represents a list of ignored selector regexp
	// This parameter can be used to exclude selectors from probes
	IgnoredSelectorsRegexp []string

	// IgnoredGroupsRegexp represents a list of ignored group regexp
	// This parameter can be used to exclude groups and therefore a set of selectors from probes
	IgnoredGroupsRegexp []string
}

// PrometheusRulesChecker represents linting PromQL logic.
type PrometheusRulesChecker struct {
	// probe implements Prober
	probe Prober

	// parser is the shared PromQL parser used for expression and selector parsing
	parser promql.Parser

	// options
	ignoredSelectorsRegexp []*regexp.Regexp
	ignoredGroupsRegexp    []*regexp.Regexp
}

// RuleGroup models a rule group that contains a set of recording and alerting rules.
type RuleGroup struct {
	// Name represents the name of the rule group
	Name string `json:"name"`

	// File represents the name of the rule group
	File string `json:"file"`

	// Rules represents a list of Rule
	Rules []Rule `json:"rules"`
}

// Rule describes an alerting or recording rule.
type Rule struct {
	// Name represents the checked recording rule or alert name
	Name string `json:"name"`

	// Expression represents the PromQL expression string
	Expression string `json:"expr"`
}

// CheckResult represents a check result.
type CheckResult struct {
	// File represents the checked file name
	File string

	// Group represents the checked group name
	Group string

	// Name represents the checked recording rule or alert name
	Name string

	// Expression represents the PromQL expression string
	Expression string

	// Results represents a list of PromQL selectors which successfully returned a result value
	Results []string

	// NoResults represents a list of PromQL selectors which did not return any result value
	NoResults []string
}

// NewPrometheusRulesChecker returns PrometheusRulesChecker.
func NewPrometheusRulesChecker(config PrometheusRulesCheckerConfig, client prometheusv1.API) (*PrometheusRulesChecker, error) {
	ignoredSelectors, err := compilePatterns(config.IgnoredSelectorsRegexp)
	if err != nil {
		return nil, fmt.Errorf("invalid ignore-selector pattern: %w", err)
	}
	ignoredGroups, err := compilePatterns(config.IgnoredGroupsRegexp)
	if err != nil {
		return nil, fmt.Errorf("invalid ignore-group pattern: %w", err)
	}
	return &PrometheusRulesChecker{
		probe: newPrometheusProbe(
			config.PrometheusURL,
			client,
		),
		parser:                 promql.NewParser(promql.Options{}),
		ignoredSelectorsRegexp: ignoredSelectors,
		ignoredGroupsRegexp:    ignoredGroups,
	}, nil
}

// compilePatterns compiles the given regexp patterns once at construction.
func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", p, err)
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

// CheckRuleGroups checks Prometheus rule groups.
// CheckRuleGroups returns a list of CheckResult.
func (prc *PrometheusRulesChecker) CheckRuleGroups(ctx context.Context, groups []RuleGroup) ([]CheckResult, error) {
	results := []CheckResult{}
	for _, g := range groups {
		if isIgnoredGroup(prc.ignoredGroupsRegexp, g.Name) {
			continue
		}
		res, err := prc.CheckRuleGroup(ctx, g)
		if err != nil {
			return results, err
		}
		results = append(results, res...)
	}
	return results, nil
}

// CheckRuleGroup checks a single rule group.
// CheckRuleGroup returns a list of CheckResult.
func (prc *PrometheusRulesChecker) CheckRuleGroup(ctx context.Context, group RuleGroup) ([]CheckResult, error) {
	var (
		mu      sync.Mutex
		results = make([]CheckResult, 0, len(group.Rules))
		eg      errgroup.Group
	)
	for _, rule := range group.Rules {
		eg.Go(func() error {
			success, failed, err := prc.probeSelectorResults(ctx, rule.Expression)
			if err != nil {
				return fmt.Errorf("rule %q: %w", rule.Name, err)
			}
			mu.Lock()
			results = append(results, CheckResult{
				File:       group.File,
				Name:       rule.Name,
				Group:      group.Name,
				Expression: rule.Expression,
				Results:    success,
				NoResults:  failed,
			})
			mu.Unlock()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

func isIgnoredGroup(patterns []*regexp.Regexp, group string) bool {
	return isIgnored(patterns, group)
}

func isIgnoredSelector(patterns []*regexp.Regexp, selector string) bool {
	return isIgnored(patterns, selector)
}

// isIgnored checks whether s matches any of the pre-compiled patterns.
// isIgnored returns true if s matches, false otherwise.
func isIgnored(patterns []*regexp.Regexp, s string) bool {
	return slices.ContainsFunc(patterns, func(re *regexp.Regexp) bool { return re.MatchString(s) })
}

// probeSelectorResults probes the given PromQL expression string for selectors without a result value.
// probeSelectorResults returns a list of successful selectors and failed selectors.
func (prc *PrometheusRulesChecker) probeSelectorResults(ctx context.Context, promqlExpression string) ([]string, []string, error) {
	selectorsWithoutResult := []string{}
	selectorsWithResult := []string{}

	selectors, err := getVectorSelectors(prc.parser, promqlExpression)
	if err != nil {
		return selectorsWithResult, selectorsWithoutResult, fmt.Errorf("getVectorSelectors failed: %w", err)
	}

	if len(selectors) == 0 {
		return selectorsWithResult, selectorsWithoutResult, nil
	}

	for _, selector := range selectors {
		if err := ctx.Err(); err != nil {
			return selectorsWithResult, selectorsWithoutResult, err
		}

		// we can move on if this selector is ignored
		if isIgnoredSelector(prc.ignoredSelectorsRegexp, selector) {
			continue
		}

		matchers, err := prc.parser.ParseMetricSelector(selector)
		if err != nil {
			return selectorsWithResult, selectorsWithoutResult, err
		}
		if ignoreMatchers(matchers) {
			continue
		}
		val, err := prc.probe.ProbeSelector(ctx, selector)
		if err != nil {
			return selectorsWithResult, selectorsWithoutResult, err
		}
		if val < 1 {
			selectorsWithoutResult = append(selectorsWithoutResult, selector)
		} else {
			selectorsWithResult = append(selectorsWithResult, selector)
		}
	}
	return selectorsWithResult, selectorsWithoutResult, nil
}

// visit is a helper struct to traverse a PromQL expression's abstract syntax tree.
type visit struct {
	vectorSelectors []string
}

// Visit implements Visitor interface.
func (v *visit) Visit(node promql.Node, _ []promql.Node) (promql.Visitor, error) {
	if node == nil {
		return v, nil
	}
	switch n := node.(type) {
	case *promql.VectorSelector:
		vs := promql.VectorSelector{
			Name:          n.Name,
			LabelMatchers: n.LabelMatchers,
		}
		v.vectorSelectors = append(v.vectorSelectors, vs.String())
	}
	return v, nil
}

// getVectorSelectors returns a list of vectorSelectors parsed from the given query.
func getVectorSelectors(p promql.Parser, promqlExpression string) ([]string, error) {
	expr, err := p.ParseExpr(promqlExpression)
	if err != nil {
		return nil, fmt.Errorf("promql parse error: %w", err)
	}
	v := &visit{
		vectorSelectors: make([]string, 0),
	}
	_ = promql.Walk(v, expr, nil)
	return v.vectorSelectors, nil
}

// ignoreMatchers checks whether the given matchers are ignored.
// ignoreMatchers returns true if the matchers are ignored, false otherwise.
func ignoreMatchers(matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if m.Name != "__name__" {
			continue
		}
		switch m.Value {
		case "ALERTS":
			return true
		case "ALERTS_FOR_STATE":
			return true
		}
	}
	return false
}
