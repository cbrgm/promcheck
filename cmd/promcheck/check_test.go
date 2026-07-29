package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	promql "github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"

	"github.com/cbrgm/promcheck/promcheck"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeChecker struct{ res []promcheck.CheckResult }

func (f *fakeChecker) CheckRuleGroup(_ context.Context, _ promcheck.RuleGroup) ([]promcheck.CheckResult, error) {
	return f.res, nil
}

type fakeReporter struct {
	sections int
	dumped   bool
}

func (r *fakeReporter) AddSection(_, _, _, _ string, _, _ []string) { r.sections++ }
func (r *fakeReporter) AddTotalCheckedGroups(int)                   {}
func (r *fakeReporter) Dump() error                                 { r.dumped = true; return nil }

type staticSource struct{ groups []promcheck.RuleGroup }

func (s staticSource) load(context.Context) ([]promcheck.RuleGroup, error) { return s.groups, nil }
func (s staticSource) name() string                                        { return "static" }

func TestRunCheck_EmptyReturnsErrNoRuleGroups(t *testing.T) {
	app := &promcheckApp{check: &fakeChecker{}, report: &fakeReporter{}, logger: newTestLogger()}
	err := app.runCheck(t.Context(), staticSource{groups: nil})
	require.ErrorIs(t, err, ErrNoRuleGroups)
}

func TestRunCheck_AddsSectionsAndDumps(t *testing.T) {
	rep := &fakeReporter{}
	app := &promcheckApp{
		check:  &fakeChecker{res: []promcheck.CheckResult{{Name: "r", Results: []string{`up`}}}},
		report: rep,
		logger: newTestLogger(),
	}
	src := staticSource{groups: []promcheck.RuleGroup{{Name: "g", Rules: []promcheck.Rule{{Name: "r", Expression: "up"}}}}}
	require.NoError(t, app.runCheck(t.Context(), src))
	require.Equal(t, 1, rep.sections)
	require.True(t, rep.dumped)
}

func TestCheckRulesFromRuleFiles_EmptyReturnsError(t *testing.T) {
	app := &promcheckApp{
		optFilesRegexp: "testdata/does-not-match-*.yaml",
		logger:         newTestLogger(),
	}
	err := app.checkRulesFromRuleFiles(t.Context())
	require.ErrorIs(t, err, ErrNoRuleGroups)
}

func TestProcessFile_ParsesRecordsAndAlerts(t *testing.T) {
	p := promql.NewParser(promql.Options{})
	groups, err := processFile(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "testdata/rules_basic.yaml")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Rules, 2)
	names := []string{groups[0].Rules[0].Name, groups[0].Rules[1].Name}
	require.ElementsMatch(t, []string{"HighLatency", "job:up:sum"}, names)
}

func TestProcessFile_UTF8Names(t *testing.T) {
	p := promql.NewParser(promql.Options{})
	groups, err := processFile(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "testdata/rules_utf8.yaml")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, "SlowRoute", groups[0].Rules[0].Name)
}
