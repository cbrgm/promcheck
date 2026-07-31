package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
)

func TestOutputFormatEnum_RejectsCSV(t *testing.T) {
	var cfg config
	parser, err := kong.New(&cfg, kong.Name("promcheck"))
	require.NoError(t, err)
	_, err = parser.Parse([]string{"--output.format", "csv"})
	require.Error(t, err, "csv must not be an accepted output format")
}

func TestNewLogger_Levels(t *testing.T) {
	require.NotNil(t, newLogger(false, "info"))
	require.NotNil(t, newLogger(true, "debug"))
}

func TestConfig_ConcurrencyDefaultAndDelayRemoved(t *testing.T) {
	var cfg config
	parser, err := kong.New(&cfg, kong.Name("promcheck"))
	require.NoError(t, err)
	_, err = parser.Parse(nil)
	require.NoError(t, err)
	require.Equal(t, 8, cfg.CheckConcurrency)

	// --check.delay must no longer be accepted
	_, err = parser.Parse([]string{"--check.delay", "0.5"})
	require.Error(t, err)
}

func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil error completes ok", nil, exitOK},
		{"strict findings", ErrStrictFindings, exitFindings},
		{"wrapped strict findings", fmt.Errorf("check: %w", ErrStrictFindings), exitFindings},
		{"no rule groups is a usage error", ErrNoRuleGroups, exitUsage},
		{"generic error is a runtime failure", errors.New("boom"), exitRuntime},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, exitCodeFor(tt.err))
		})
	}
}
