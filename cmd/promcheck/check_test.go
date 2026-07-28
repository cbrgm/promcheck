package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/go-kit/log"
	promql "github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
)

func newTestLogger() log.Logger {
	return log.NewLogfmtLogger(io.Discard)
}

func TestCheckRulesFromRuleFiles_EmptyReturnsError(t *testing.T) {
	app := &promcheckApp{
		optFilesRegexp: "testdata/does-not-match-*.yaml",
		logger:         newTestLogger(),
	}
	err := app.checkRulesFromRuleFiles()
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
