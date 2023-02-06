package promcheck

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/prometheus/prometheus/model/labels"
	promql "github.com/prometheus/prometheus/promql/parser"
)

// PrometheusRulesCheckerConfig represents PrometheusRulesChecker configuration.
type PrometheusRulesCheckerConfig struct {
	// ProbeDelay represents the delay between selector probes
	ProbeDelay time.Duration

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

	// options
	ignoredSelectorsRegexp []string
	ignoredGroupsRegexp    []string
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
func NewPrometheusRulesChecker(config PrometheusRulesCheckerConfig, client prometheusv1.API) *PrometheusRulesChecker {
	return &PrometheusRulesChecker{
		probe: newPrometheusProbe(
			config.ProbeDelay,
			config.PrometheusURL,
			client,
		),
		ignoredSelectorsRegexp: config.IgnoredSelectorsRegexp,
		ignoredGroupsRegexp:    config.IgnoredGroupsRegexp,
	}
}

// CheckRuleGroups checks Prometheus rule groups.
// CheckRuleGroups returns a list of CheckResult.
func (prc *PrometheusRulesChecker) CheckRuleGroups(groups []RuleGroup) ([]CheckResult, error) {
	results := []CheckResult{}
	for _, g := range groups {
		if isIgnoredGroup(prc.ignoredGroupsRegexp, g.Name) {
			continue
		}
		res, err := prc.CheckRuleGroup(g)
		if err != nil {
			return results, err
		}
		results = append(results, res...)
	}
	return results, nil
}

// CheckRuleGroup checks a single rule group.
// CheckRuleGroup returns a list of CheckResult.
func (prc *PrometheusRulesChecker) CheckRuleGroup(group RuleGroup) ([]CheckResult, error) {
	var wg sync.WaitGroup
	results := make([]CheckResult, 0, len(group.Rules))
	resultCh := make(chan CheckResult, len(group.Rules))
	for _, rule := range group.Rules {
		rule := rule
		wg.Add(1)
		go func() {
			defer wg.Done()
			success, failed, err := prc.probeSelectorResults(rule.Expression)
			if err != nil {
				return
			}
			resultCh <- CheckResult{
				File:       group.File,
				Name:       rule.Name,
				Group:      group.Name,
				Expression: rule.Expression,
				Results:    success,
				NoResults:  failed,
			}
		}()
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()
	for result := range resultCh {
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
