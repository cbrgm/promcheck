package promcheck

import (
	"github.com/prometheus/prometheus/model/rulefmt"
	promql "github.com/prometheus/prometheus/promql/parser"
	"reflect"
	"testing"
)

func TestPrometheusRulesChecker_CheckRuleGroups(t *testing.T) {
	type fields struct {
		probe                  Probe
		ignoredSelectorsRegexp []string
		ignoredGroupsRegexp    []string
	}
	type args struct {
		fileName string
		groups   []rulefmt.RuleGroup
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []CheckResult
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prc := &PrometheusRulesChecker{
				probe:                  tt.fields.probe,
				ignoredSelectorsRegexp: tt.fields.ignoredSelectorsRegexp,
				ignoredGroupsRegexp:    tt.fields.ignoredGroupsRegexp,
			}
			got, err := prc.CheckRuleGroups(tt.args.fileName, tt.args.groups)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckRuleGroups() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CheckRuleGroups() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrometheusRulesChecker_checkRuleGroup(t *testing.T) {
	type fields struct {
		probe                  Probe
		ignoredSelectorsRegexp []string
		ignoredGroupsRegexp    []string
	}
	type args struct {
		fileName string
		group    rulefmt.RuleGroup
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []CheckResult
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prc := &PrometheusRulesChecker{
				probe:                  tt.fields.probe,
				ignoredSelectorsRegexp: tt.fields.ignoredSelectorsRegexp,
				ignoredGroupsRegexp:    tt.fields.ignoredGroupsRegexp,
			}
			got, err := prc.checkRuleGroup(tt.args.fileName, tt.args.group)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkRuleGroup() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("checkRuleGroup() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrometheusRulesChecker_probeSelectorResults(t *testing.T) {
	type fields struct {
		probe                  Probe
		ignoredSelectorsRegexp []string
		ignoredGroupsRegexp    []string
	}
	type args struct {
		promqlExpression string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []string
		want1   []string
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prc := &PrometheusRulesChecker{
				probe:                  tt.fields.probe,
				ignoredSelectorsRegexp: tt.fields.ignoredSelectorsRegexp,
				ignoredGroupsRegexp:    tt.fields.ignoredGroupsRegexp,
			}
			got, got1, err := prc.probeSelectorResults(tt.args.promqlExpression)
			if (err != nil) != tt.wantErr {
				t.Errorf("probeSelectorResults() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("probeSelectorResults() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("probeSelectorResults() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_getVectorSelectorsFromExpression(t *testing.T) {
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getVectorSelectorsFromExpression(tt.args.promqlExpression)
			if (err != nil) != tt.wantErr {
				t.Errorf("getVectorSelectorsFromExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getVectorSelectorsFromExpression() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ignoreMatchers(t *testing.T) {
	toTest := map[string]bool{
		"ALERTS{kubernetes=\"foo\"}":           true,
		"ALERTS_FOR_STATE{kubernetes=\"foo\"}": true,
		"up{kubernetes=\"foo\"}":               false,
	}
	for expression, want := range toTest {
		matchers, err := promql.ParseMetricSelector(expression)
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
