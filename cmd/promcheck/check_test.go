package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promql "github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"

	"github.com/cbrgm/promcheck/internal/checker"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeChecker struct {
	res []checker.CheckResult

	// ignoredGroups names the groups IsIgnoredGroup reports as ignored.
	ignoredGroups []string

	mu            sync.Mutex
	checkedGroups []string
}

func (f *fakeChecker) IsIgnoredGroup(name string) bool {
	return slices.Contains(f.ignoredGroups, name)
}

func (f *fakeChecker) CheckRuleGroup(_ context.Context, group checker.RuleGroup) ([]checker.CheckResult, error) {
	f.mu.Lock()
	f.checkedGroups = append(f.checkedGroups, group.Name)
	f.mu.Unlock()
	return f.res, nil
}

type fakeReporter struct {
	sections    int
	groupsTotal int
	dumped      bool
}

func (r *fakeReporter) AddSection(_, _, _, _ string, _, _ []string) { r.sections++ }
func (r *fakeReporter) AddTotalCheckedGroups(count int)             { r.groupsTotal = count }
func (r *fakeReporter) Dump() error                                 { r.dumped = true; return nil }

type staticSource struct{ groups []checker.RuleGroup }

func (s staticSource) load(context.Context) ([]checker.RuleGroup, error) { return s.groups, nil }
func (s staticSource) name() string                                      { return "static" }

func TestRunCheck_EmptyReturnsErrNoRuleGroups(t *testing.T) {
	app := &promcheckApp{check: &fakeChecker{}, report: &fakeReporter{}, logger: newTestLogger()}
	err := app.runCheck(t.Context(), staticSource{groups: nil})
	require.ErrorIs(t, err, ErrNoRuleGroups)
}

func TestRunCheck_AddsSectionsAndDumps(t *testing.T) {
	rep := &fakeReporter{}
	app := &promcheckApp{
		check:  &fakeChecker{res: []checker.CheckResult{{Name: "r", Results: []string{`up`}}}},
		report: rep,
		logger: newTestLogger(),
	}
	src := staticSource{groups: []checker.RuleGroup{{Name: "g", Rules: []checker.Rule{{Name: "r", Expression: "up"}}}}}
	require.NoError(t, app.runCheck(t.Context(), src))
	require.Equal(t, 1, rep.sections)
	require.True(t, rep.dumped)
}

func TestRunCheck_SkipsIgnoredGroups(t *testing.T) {
	rep := &fakeReporter{}
	fc := &fakeChecker{
		res:           []checker.CheckResult{{Name: "r", Results: []string{`up`}}},
		ignoredGroups: []string{"ignore-me"},
	}
	app := &promcheckApp{check: fc, report: rep, logger: newTestLogger()}
	src := staticSource{groups: []checker.RuleGroup{
		{Name: "ignore-me", Rules: []checker.Rule{{Name: "r1", Expression: "up"}}},
		{Name: "keep", Rules: []checker.Rule{{Name: "r2", Expression: "up"}}},
	}}
	require.NoError(t, app.runCheck(t.Context(), src))

	require.Equal(t, []string{"keep"}, fc.checkedGroups, "ignored group must not be probed")
	require.Equal(t, 1, rep.groupsTotal, "ignored group must not be counted")
	require.Equal(t, 1, rep.sections, "ignored group must not produce sections")
}

func TestRunCheck_StrictModeDoesNotExitInExporterMode(t *testing.T) {
	rep := &fakeReporter{}
	app := &promcheckApp{
		check:                  &fakeChecker{res: []checker.CheckResult{{Name: "r", NoResults: []string{`up`}}}},
		report:                 rep,
		logger:                 newTestLogger(),
		optStrictMode:          true,
		optExporterModeEnabled: true,
	}
	src := staticSource{groups: []checker.RuleGroup{{Name: "g", Rules: []checker.Rule{{Name: "r", Expression: "up"}}}}}

	// Must return normally (not os.Exit the test process) even though there
	// are NoResults and strict mode is on, because the exporter mode is a
	// long-running process that must not die on the first dead rule.
	err := app.runCheck(t.Context(), src)
	require.NoError(t, err)
	require.True(t, rep.dumped)
}

// TestRunCheck_StrictModeReturnsErrStrictFindingsOneShot verifies that
// one-shot --strict mode reports its findings and returns the
// ErrStrictFindings sentinel instead of exiting the process directly; main()
// is the only place that turns errors into a process exit code.
func TestRunCheck_StrictModeReturnsErrStrictFindingsOneShot(t *testing.T) {
	rep := &fakeReporter{}
	app := &promcheckApp{
		check:         &fakeChecker{res: []checker.CheckResult{{Name: "r", NoResults: []string{`up`}}}},
		report:        rep,
		logger:        newTestLogger(),
		optStrictMode: true,
	}
	src := staticSource{groups: []checker.RuleGroup{{Name: "g", Rules: []checker.Rule{{Name: "r", Expression: "up"}}}}}

	err := app.runCheck(t.Context(), src)
	require.ErrorIs(t, err, ErrStrictFindings)
	require.True(t, rep.dumped, "report must still be dumped before returning the sentinel")
}

// TestRunCheck_StrictModeExitsOneShot verifies the full contract end to end:
// a one-shot --strict run with a dead rule exits the process with status 1.
// Since os.Exit terminates the process, this is exercised via a subprocess
// that mirrors what main() does with the runCheck result.
func TestRunCheck_StrictModeExitsOneShot(t *testing.T) {
	if os.Getenv("PROMCHECK_TEST_STRICT_EXIT") == "1" {
		rep := &fakeReporter{}
		app := &promcheckApp{
			check:         &fakeChecker{res: []checker.CheckResult{{Name: "r", NoResults: []string{`up`}}}},
			report:        rep,
			logger:        newTestLogger(),
			optStrictMode: true,
		}
		src := staticSource{groups: []checker.RuleGroup{{Name: "g", Rules: []checker.Rule{{Name: "r", Expression: "up"}}}}}
		err := app.runCheck(t.Context(), src)
		os.Exit(exitCodeFor(err))
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRunCheck_StrictModeExitsOneShot")
	cmd.Env = append(os.Environ(), "PROMCHECK_TEST_STRICT_EXIT=1")
	err := cmd.Run()

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 1, exitErr.ExitCode())
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

func TestInstanceSource_PassesMatchersToRulesAPI(t *testing.T) {
	var gotMatch []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		gotMatch = r.Form["match[]"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"groups":[]}}`))
	}))
	defer srv.Close()

	client, err := api.NewClient(api.Config{Address: srv.URL})
	require.NoError(t, err)
	src := instanceSource{api: prometheusv1.NewAPI(client), matchers: []string{`{team="infra"}`}}
	_, err = src.load(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{`{team="infra"}`}, gotMatch)
}

func TestProcessFile_UTF8Names(t *testing.T) {
	p := promql.NewParser(promql.Options{})
	groups, err := processFile(p, slog.New(slog.NewTextHandler(io.Discard, nil)), "testdata/rules_utf8.yaml")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, "SlowRoute", groups[0].Rules[0].Name)
}
