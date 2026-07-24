package main

import (
	"io"
	"testing"

	"github.com/go-kit/log"
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
