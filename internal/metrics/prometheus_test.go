package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPrometheus_SetBuildInfo(t *testing.T) {
	p := NewPrometheus(DefaultOptions())

	p.SetBuildInfo("1.2.3", "abc", "go1.26")

	if got := testutil.CollectAndCount(p.buildInfoGaugeM); got != 1 {
		t.Fatalf("expected 1 build_info series, got %d", got)
	}

	value := testutil.ToFloat64(p.buildInfoGaugeM.WithLabelValues("1.2.3", "abc", "go1.26"))
	if value != 1 {
		t.Fatalf("expected build_info value 1, got %v", value)
	}
}

func TestPrometheus_SetLastRunTimestamp(t *testing.T) {
	p := NewPrometheus(DefaultOptions())

	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	p.SetLastRunTimestamp(now)

	if got := testutil.ToFloat64(p.lastRunTimestampM); got != float64(now.Unix()) {
		t.Fatalf("expected %v, got %v", float64(now.Unix()), got)
	}
}

func TestPrometheus_SetRunDuration(t *testing.T) {
	p := NewPrometheus(DefaultOptions())

	p.SetRunDuration(2500 * time.Millisecond)

	if got := testutil.ToFloat64(p.runDurationM); got != 2.5 {
		t.Fatalf("expected 2.5, got %v", got)
	}
}

func TestPrometheus_IncRunErrors(t *testing.T) {
	p := NewPrometheus(DefaultOptions())

	p.IncRunErrors()
	p.IncRunErrors()

	if got := testutil.ToFloat64(p.runErrorsTotalM); got != 2 {
		t.Fatalf("expected 2, got %v", got)
	}
}

// TestPrometheus_Prefix confirms the new health metrics pick up a configured
// prefix exactly like the existing gauges do: namespace becomes the
// (dot-trimmed) prefix and is prepended to the bare metric name, with no
// implicit "promcheck" component. The existing rule_groups_total gauge
// additionally carries the "validation" subsystem, while the new run-health
// metrics intentionally do not use a subsystem.
func TestPrometheus_Prefix(t *testing.T) {
	p := NewPrometheus(Options{Prefix: "myprefix"})

	p.SetRuleGroupsTotal(3)
	p.IncRunErrors()

	expected := `
# HELP myprefix_run_errors_total Total number of check cycles that returned an error.
# TYPE myprefix_run_errors_total counter
myprefix_run_errors_total 1
`
	if err := testutil.CollectAndCompare(p.runErrorsTotalM, strings.NewReader(expected), "myprefix_run_errors_total"); err != nil {
		t.Fatalf("unexpected collecting result:\n%s", err)
	}

	expectedGroups := `
# HELP myprefix_validation_rule_groups_total Total number of evaluated rule groups.
# TYPE myprefix_validation_rule_groups_total gauge
myprefix_validation_rule_groups_total 3
`
	if err := testutil.CollectAndCompare(p.ruleGroupsGaugeM, strings.NewReader(expectedGroups), "myprefix_validation_rule_groups_total"); err != nil {
		t.Fatalf("unexpected collecting result:\n%s", err)
	}
}

// TestPrometheus_PrefixWithTrailingDot mirrors the TrimSuffix(".") handling
// in NewPrometheus for the new metrics.
func TestPrometheus_PrefixWithTrailingDot(t *testing.T) {
	p := NewPrometheus(Options{Prefix: "myprefix."})

	p.IncRunErrors()

	expected := `
# HELP myprefix_run_errors_total Total number of check cycles that returned an error.
# TYPE myprefix_run_errors_total counter
myprefix_run_errors_total 1
`
	if err := testutil.CollectAndCompare(p.runErrorsTotalM, strings.NewReader(expected), "myprefix_run_errors_total"); err != nil {
		t.Fatalf("unexpected collecting result:\n%s", err)
	}
}
