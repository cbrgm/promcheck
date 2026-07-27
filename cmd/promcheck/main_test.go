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
