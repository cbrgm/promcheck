package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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

func TestConfig_MetricsProfileAndRuntimeDefaultOff(t *testing.T) {
	var cfg config
	parser, err := kong.New(&cfg, kong.Name("promcheck"))
	require.NoError(t, err)
	_, err = parser.Parse(nil)
	require.NoError(t, err)

	require.False(t, cfg.ExporterEnableProfiling, "pprof profiling must be opt-in via --metrics.profile")
	require.False(t, cfg.ExporterEnableRuntimeMetrics, "runtime metrics must be opt-in via --metrics.runtime")
}

// TestMain_HelpExitsZero and TestMain_InvalidFlagExitsUsage verify that
// kong's own exits (which happen inside kong.Parse, before runMain ever
// runs) are routed through the kong.Exit(...) hook wired up in main(): a
// parse error must land on exitUsage (2), while --help must still exit 0.
// Since these exercise main() itself and os.Exit terminates the process,
// they run in a subprocess, mirroring TestRunCheck_StrictModeExitsOneShot.
func TestMain_HelpExitsZero(t *testing.T) {
	if os.Getenv("PROMCHECK_TEST_MAIN_HELP") == "1" {
		os.Args = []string{"promcheck", "--help"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMain_HelpExitsZero")
	cmd.Env = append(os.Environ(), "PROMCHECK_TEST_MAIN_HELP=1")
	err := cmd.Run()
	require.NoError(t, err, "--help must exit 0")
}

func TestMain_InvalidFlagExitsUsage(t *testing.T) {
	if os.Getenv("PROMCHECK_TEST_MAIN_BADFLAG") == "1" {
		os.Args = []string{"promcheck", "--output.format=bogus"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMain_InvalidFlagExitsUsage")
	cmd.Env = append(os.Environ(), "PROMCHECK_TEST_MAIN_BADFLAG=1")
	err := cmd.Run()

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, exitUsage, exitErr.ExitCode())
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
