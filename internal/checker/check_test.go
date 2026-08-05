package checker

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	promql "github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
)

// mustCompileAll compiles each pattern for use in the ignore tests.
func mustCompileAll(patterns []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}

func Test_getVectorSelectors(t *testing.T) {
	type args struct {
		promqlExpression string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "must parse selectors",
			args: args{
				promqlExpression: `
				  sum by (namespace, pod) (
					max by(namespace, pod) (
					  kube_pod_status_phase{job="kube-state-metrics", phase=~"Pending|Unknown"}
					) * on(namespace, pod) group_left(owner_kind) topk by(namespace, pod) (
					  1, max by(namespace, pod, owner_kind) (kube_pod_owner{owner_kind!="Job"})
					)
				  ) > 0
				`,
			},
			want: []string{
				`kube_pod_status_phase{job="kube-state-metrics",phase=~"Pending|Unknown"}`,
				`kube_pod_owner{owner_kind!="Job"}`,
			},
			wantErr: false,
		},
		{
			name: "must parse selectors",
			args: args{
				promqlExpression: `
				  absent(up{job="kube-controller-manager"} == 1)
				`,
			},
			want: []string{
				`up{job="kube-controller-manager"}`,
			},
			wantErr: false,
		},
	}
	p := promql.NewParser(promql.Options{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getVectorSelectors(p, tt.args.promqlExpression)
			if (err != nil) != tt.wantErr {
				t.Errorf("getVectorSelectors() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getVectorSelectors() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ignoreMatchers(t *testing.T) {
	p := promql.NewParser(promql.Options{})
	toTest := map[string]bool{
		"ALERTS{kubernetes=\"foo\"}":           true,
		"ALERTS_FOR_STATE{kubernetes=\"foo\"}": true,
		"up{kubernetes=\"foo\"}":               false,
	}
	for expression, want := range toTest {
		matchers, err := p.ParseMetricSelector(expression)
		if err != nil {
			panic(err)
		}
		if ignoreMatchers(matchers) != want {
			t.Errorf("%s not ignored", expression)
		}
	}
}

func Test_isIgnored(t *testing.T) {
	type args struct {
		ignoredRegexp []string
		selector      string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "must be ignored",
			args: args{
				ignoredRegexp: []string{"foo"},
				selector:      "bar",
			},
			want: false,
		},
		{
			name: "must not ignored",
			args: args{
				ignoredRegexp: []string{"foo"},
				selector:      "foo",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isIgnored(mustCompileAll(tt.args.ignoredRegexp), tt.args.selector); got != tt.want {
				t.Errorf("isIgnored() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isIgnoredGroup(t *testing.T) {
	type args struct {
		ignoredRegexp []string
		group         string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "must ignore group",
			args: args{
				ignoredRegexp: []string{
					"kubernetes-apps",
				},
				group: "kubernetes-apps-foo",
			},
			want: true,
		},
		{
			name: "must not ignore group",
			args: args{
				ignoredRegexp: []string{
					"kubernetes-apps",
				},
				group: "kubernetes",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isIgnoredGroup(mustCompileAll(tt.args.ignoredRegexp), tt.args.group); got != tt.want {
				t.Errorf("isIgnoredGroup() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrometheusRulesChecker_IsIgnoredGroup(t *testing.T) {
	prc, err := NewPrometheusRulesChecker(PrometheusRulesCheckerConfig{
		IgnoredGroupsRegexp: []string{"^ignore-me$"},
	}, nil)
	require.NoError(t, err)

	require.True(t, prc.IsIgnoredGroup("ignore-me"))
	require.False(t, prc.IsIgnoredGroup("keep"))
}

func Test_isIgnoredSelector(t *testing.T) {
	type args struct {
		ignoredRegexp []string
		selector      string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "must be ignored",
			args: args{
				ignoredRegexp: []string{
					`test_metric1{foo="bar"}`,
					`test_metric1{foo="boo"}`,
				},
				selector: `test_metric1{foo="bar"}`,
			},
			want: true,
		},
		{
			name: "must be ignored",
			args: args{
				ignoredRegexp: []string{
					`test_metric1*`,
				},
				selector: `test_metric1{foo="bar"}`,
			},
			want: true,
		},
		{
			name: "must be ignored",
			args: args{
				ignoredRegexp: []string{
					`bar`,
				},
				selector: `test_metric1{foo="bar"}`,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isIgnoredSelector(mustCompileAll(tt.args.ignoredRegexp), tt.args.selector); got != tt.want {
				t.Errorf("isIgnoredSelector() = %v, want %v", got, tt.want)
			}
		})
	}
}

// fakeProber is a test helper implementing Prober interface
type fakeProber struct {
	// values maps a selector string to the value ProbeSelector returns
	values map[string]float64
	// err, if set, is returned for every probe
	err error
	// calls records every selector passed to ProbeSelector, in order
	calls []string
	// tsCalls records the evaluation timestamp passed to ProbeSelector for
	// each call, in the same order as calls
	tsCalls []time.Time
}

func (f *fakeProber) ProbeSelector(ctx context.Context, selector string, ts time.Time) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	f.calls = append(f.calls, selector)
	f.tsCalls = append(f.tsCalls, ts)
	if f.err != nil {
		return 0, f.err
	}
	return f.values[selector], nil
}

func TestProbeSelectorResults_ContinuesAfterIgnoredMatcher(t *testing.T) {
	fp := &fakeProber{values: map[string]float64{
		`up{job="x"}`: 1, // has a result
	}}
	prc := &PrometheusRulesChecker{probe: fp, parser: promql.NewParser(promql.Options{})}

	// ALERTS{...} is ignored; up{job="x"} must still be probed.
	success, failed, err := prc.probeSelectorResults(t.Context(), time.Now(), `ALERTS{alertname="Foo"} or up{job="x"}`)
	require.NoError(t, err)
	require.Contains(t, fp.calls, `up{job="x"}`, "selector after the ignored ALERTS selector must still be probed")
	require.Equal(t, []string{`up{job="x"}`}, success)
	require.Empty(t, failed)
}

func TestCheckRuleGroup_PropagatesProbeError(t *testing.T) {
	fp := &fakeProber{err: errors.New("boom")}
	prc := &PrometheusRulesChecker{probe: fp, parser: promql.NewParser(promql.Options{})}
	group := RuleGroup{
		Name:  "g",
		Rules: []Rule{{Name: "r", Expression: `up{job="x"}`}},
	}
	_, err := prc.CheckRuleGroup(t.Context(), group)
	require.Error(t, err)
}

func TestCheckRuleGroup_AppliesQueryOffset(t *testing.T) {
	fp := &fakeProber{values: map[string]float64{`up{job="x"}`: 1}}
	prc := &PrometheusRulesChecker{probe: fp, parser: promql.NewParser(promql.Options{})}
	group := RuleGroup{
		Name:        "g",
		QueryOffset: 5 * time.Minute,
		Rules:       []Rule{{Name: "r", Expression: `up{job="x"}`}},
	}

	before := time.Now()
	_, err := prc.CheckRuleGroup(t.Context(), group)
	after := time.Now()
	require.NoError(t, err)

	require.Len(t, fp.tsCalls, 1)
	got := fp.tsCalls[0]
	wantEarliest := before.Add(-group.QueryOffset).Add(-2 * time.Second)
	wantLatest := after.Add(-group.QueryOffset).Add(2 * time.Second)
	require.True(t, !got.Before(wantEarliest) && !got.After(wantLatest),
		"probe ts %v not within tolerance of now-%v (want between %v and %v)", got, group.QueryOffset, wantEarliest, wantLatest)
}

// countingProber records the maximum number of ProbeSelector calls observed
// running concurrently, using atomics so the test itself is race-free.
type countingProber struct {
	active atomic.Int64
	max    atomic.Int64
}

func (c *countingProber) ProbeSelector(_ context.Context, _ string, _ time.Time) (float64, error) {
	n := c.active.Add(1)
	for {
		old := c.max.Load()
		if n <= old || c.max.CompareAndSwap(old, n) {
			break
		}
	}
	time.Sleep(time.Millisecond)
	c.active.Add(-1)
	return 1, nil
}

func TestCheckRuleGroup_BoundsConcurrentProbes(t *testing.T) {
	prc, err := NewPrometheusRulesChecker(PrometheusRulesCheckerConfig{MaxConcurrency: 2}, nil)
	require.NoError(t, err)

	cp := &countingProber{}
	prc.probe = cp // swap in the counting prober; sem is already wired by the constructor

	group := RuleGroup{Name: "g"}
	for i := 0; i < 10; i++ {
		group.Rules = append(group.Rules, Rule{
			Name:       fmt.Sprintf("r%d", i),
			Expression: fmt.Sprintf(`up{job="j%d"}`, i),
		})
	}

	_, err = prc.CheckRuleGroup(t.Context(), group)
	require.NoError(t, err)
	// Assert only the upper bound; never assert it reaches the limit (flaky).
	require.LessOrEqual(t, cp.max.Load(), int64(2), "concurrent probes must never exceed MaxConcurrency")
}

func TestProbeSelector_HonorsContextCancellation(t *testing.T) {
	fp := &fakeProber{} // fakeProber updated to accept ctx (see below)
	prc := &PrometheusRulesChecker{probe: fp, parser: promql.NewParser(promql.Options{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := prc.probeSelectorResults(ctx, time.Now(), `up{job="x"}`)
	require.ErrorIs(t, err, context.Canceled)
}

// siblingCancelProber lets a test prove that an error from one rule's probe
// cancels the derived context used by sibling probes in the same group.
// probeSelectorResults checks ctx.Err() at the top of its selector loop,
// before calling ProbeSelector, so if the erroring probe cancels ctx before
// the sibling goroutine reaches ProbeSelector, that goroutine would
// short-circuit at the loop guard and never observe cancellation inside
// ProbeSelector. To make the observation deterministic, the "err_metric"
// selector withholds its error until the "slow_metric" selector confirms
// (via slowStarted) that it is parked on ctx.Done() inside ProbeSelector;
// only then does cancellation propagate, so it is always observed at the
// ProbeSelector level.
type siblingCancelProber struct {
	slowStarted chan struct{} // closed once the slow probe is parked on ctx.Done()
	sawCancel   chan struct{} // closed once the slow probe observes cancellation
}

func (p *siblingCancelProber) ProbeSelector(ctx context.Context, selector string, _ time.Time) (float64, error) {
	switch selector {
	case `err_metric{job="x"}`:
		// Don't error until the slow probe is actually parked on ctx.Done(),
		// so the cancellation it triggers is observed deterministically.
		select {
		case <-p.slowStarted:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		return 0, errors.New("boom")
	case `slow_metric{job="y"}`:
		close(p.slowStarted)
		<-ctx.Done() // released only by cancellation (no timeout fallback)
		close(p.sawCancel)
		return 0, ctx.Err()
	default:
		return 0, fmt.Errorf("unexpected selector %q", selector)
	}
}

func TestCheckRuleGroup_CancelsSiblingsOnError(t *testing.T) {
	p := &siblingCancelProber{slowStarted: make(chan struct{}), sawCancel: make(chan struct{})}
	// No sem (MaxConcurrency unset) so both rules probe concurrently; a
	// MaxConcurrency of 1 would serialize them and deadlock this design.
	prc := &PrometheusRulesChecker{probe: p, parser: promql.NewParser(promql.Options{})}
	group := RuleGroup{
		Name: "g",
		Rules: []Rule{
			{Name: "erroring", Expression: `err_metric{job="x"}`},
			{Name: "slow", Expression: `slow_metric{job="y"}`},
		},
	}

	// Bound the wait so a regression (broken cancellation, leaving the slow
	// probe blocked forever) fails the test instead of hanging.
	done := make(chan error, 1)
	go func() {
		_, e := prc.CheckRuleGroup(t.Context(), group)
		done <- e
	}()

	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("CheckRuleGroup did not return; sibling cancellation likely broken")
	}

	select {
	case <-p.sawCancel:
	default:
		t.Fatal("sibling probe did not observe context cancellation")
	}
}

func TestGetVectorSelectors_UTF8Names(t *testing.T) {
	p := promql.NewParser(promql.Options{})
	// dotted metric and label names require the quoted brace form
	sel, err := getVectorSelectors(p, `sum({"http.server.duration", "http.route"="/x"})`)
	require.NoError(t, err)
	require.Len(t, sel, 1)
	// the reconstructed selector must be re-parseable
	_, err = p.ParseMetricSelector(sel[0])
	require.NoError(t, err, "reconstructed UTF-8 selector must round-trip: %q", sel[0])
}
