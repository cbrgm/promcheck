package promcheck

import (
	"errors"
	"reflect"
	"testing"

	promql "github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
)

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
			if got := isIgnored(tt.args.ignoredRegexp, tt.args.selector); got != tt.want {
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
			if got := isIgnoredGroup(tt.args.ignoredRegexp, tt.args.group); got != tt.want {
				t.Errorf("isIgnoredGroup() = %v, want %v", got, tt.want)
			}
		})
	}
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
			if got := isIgnoredSelector(tt.args.ignoredRegexp, tt.args.selector); got != tt.want {
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
}

func (f *fakeProber) ProbeSelector(selector string) (float64, error) {
	f.calls = append(f.calls, selector)
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
	success, failed, err := prc.probeSelectorResults(`ALERTS{alertname="Foo"} or up{job="x"}`)
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
	_, err := prc.CheckRuleGroup(group)
	require.Error(t, err)
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
