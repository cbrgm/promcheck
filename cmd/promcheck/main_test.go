package main

import (
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
