package promcheck

import (
	"fmt"
	"regexp"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	promql "github.com/prometheus/prometheus/promql/parser"
)

// Probe represents probe
type Probe interface {
	// ProbeSelector probes the given PromQL selector against a remote instance.
	ProbeSelector(selector string) (uint64, error)
}

// PrometheusRulesCheckerConfig represents PrometheusRulesChecker configuration.
type PrometheusRulesCheckerConfig struct {

	// ProbeDelay represents the delay between selector probes
	ProbeDelay time.Duration

	// PrometheusUrl represents the Prometheus instance url
	PrometheusUrl string

	// IgnoredSelectorsRegexp represents a list of ignored selector regexp
	// This parameter can be used to exclude selectors from probes
	IgnoredSelectorsRegexp []string

	// IgnoredGroupsRegexp represents a list of ignored group regexp
	// This parameter can be used to exclude groups and therefore a set of selectors from probes
	IgnoredGroupsRegexp []string

	// Probe implements prober
	// This parameter can be used to set custom probe logic or set a probe mock for testing.
	Probe Probe
}

// PrometheusRulesChecker represents linting PromQL logic.
type PrometheusRulesChecker struct {
	// probe implements Probe
	probe Probe

	// options
	ignoredSelectorsRegexp []string
	ignoredGroupsRegexp    []string
}

// CheckResult represents a check result
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

//NewPrometheusRulesChecker returns PrometheusRulesChecker
func NewPrometheusRulesChecker(config PrometheusRulesCheckerConfig) *PrometheusRulesChecker {
	var probe Probe
	if config.Probe == nil {
		probe = newPrometheusProbe(
			config.ProbeDelay,
			config.PrometheusUrl,
			defaultHTTPClient,
		)
	}
	prc := &PrometheusRulesChecker{
		probe:                  probe,
		ignoredSelectorsRegexp: config.IgnoredSelectorsRegexp,
		ignoredGroupsRegexp:    config.IgnoredGroupsRegexp,
	}
	return prc
}

// CheckRuleGroups checks Prometheus rule groups.
// CheckRuleGroups returns a list of CheckResult.
func (prc *PrometheusRulesChecker) CheckRuleGroups(fileName string, groups []rulefmt.RuleGroup) ([]CheckResult, error) {
	results := []CheckResult{}
	for _, g := range groups {
		if isIgnoredGroup(prc.ignoredGroupsRegexp, g.Name) {
			continue
		}
		res, err := prc.checkRuleGroup(fileName, g)
		if err != nil {
			return results, err
		}
		results = append(results, res...)
	}
	return results, nil
}

// checkRuleGroup checks a single rule group.
// checkRuleGroup returns a list of CheckResult.
func (prc *PrometheusRulesChecker) checkRuleGroup(fileName string, group rulefmt.RuleGroup) ([]CheckResult, error) {
	results := []CheckResult{}
	for _, rule := range group.Rules {
		success, failed, err := prc.probeSelectorResults(rule.Expr.Value)
		if err != nil {
			return results, err
		}

		var ruleName string
		if rule.Record.Value == "" {
			ruleName = rule.Alert.Value
		}

		if rule.Alert.Value == "" {
			ruleName = rule.Record.Value
		}

		result := CheckResult{
			File:       fileName,
			Name:       ruleName,
			Group:      group.Name,
			Expression: rule.Expr.Value,
			Results:    failed,
			NoResults:  success,
		}
		results = append(results, result)
	}
	return results, nil
}

func isIgnoredGroup(ignoredRegexp []string, group string) bool {
	return isIgnored(ignoredRegexp, group)
}

func isIgnoredSelector(ignoredRegexp []string, selector string) bool {
	return isIgnored(ignoredRegexp, selector)
}

// isIgnored checks whether the given selector matches ignoredRegexp.
// isIgnored returns true if selector matches, false otherwise.
func isIgnored(ignoredRegexp []string, selector string) bool {
	if ignoredRegexp == nil {
		return false
	}
	for _, re := range ignoredRegexp {
		isMatching, err := regexp.MatchString(re, selector)
		if err != nil {
			return false
		}
		if isMatching {
			return true
		}
	}
	return false
}

// probeSelectorResults probes the given PromQL expression string for selectors without a result value.
// probeSelectorResults returns a list of successful selectors and failed selectors.
func (prc *PrometheusRulesChecker) probeSelectorResults(promqlExpression string) ([]string, []string, error) {

	selectorsWithoutResult := []string{}
	selectorsWithResult := []string{}

	selectors, err := getVectorSelectorsFromExpression(promqlExpression)
	if err != nil {
		return selectorsWithResult, selectorsWithoutResult, fmt.Errorf("getVectorSelectorsFromExpression failed: %s", err)
	}

	if len(selectors) == 0 {
		return selectorsWithResult, selectorsWithoutResult, nil
	}

	for _, selector := range selectors {
		// we can move on if this selector is ignored
		if isIgnoredSelector(prc.ignoredSelectorsRegexp, selector) {
			continue
		}

		matchers, err := promql.ParseMetricSelector(selector)
		if err != nil {
			return selectorsWithResult, selectorsWithoutResult, err
		}
		if ignoreMatchers(matchers) {
			break
		}
		val, err := prc.probe.ProbeSelector(selector)
		if val < 1 {
			selectorsWithoutResult = append(selectorsWithoutResult, selector)
		} else {
			selectorsWithResult = append(selectorsWithResult, selector)
		}
	}
	return selectorsWithResult, selectorsWithoutResult, nil
}

// visit is a helper struct to traverse a PromQL expression's abstract syntax tree
type visit struct {
	vectorSelectors []string
}

// Visit implements Visitor interface
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

// getVectorSelectorsFromExpression returns a list of vectorSelectors parsed from the given query.
func getVectorSelectorsFromExpression(promqlExpression string) ([]string, error) {
	expr, err := promql.ParseExpr(promqlExpression)
	if err != nil {
		return nil, fmt.Errorf("promql parse error: %s", err)
	}
	v := &visit{
		vectorSelectors: make([]string, 0),
	}
	var path []promql.Node
	_ = promql.Walk(v, expr, path)
	return v.vectorSelectors, nil
}

// ignoreMatchers checks whether the given matchers are ignored.
// ignoreMatchers returns true if the matchers are ignored, false otherwise
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
