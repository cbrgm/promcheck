package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuilder_DumpJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	b := NewBuilder(WithWriter(buf))

	// TODO: Empty Builder doesn't work.
	// require.NoError(t, b.DumpJSON())
	// require.JSONEq(t, `{}`, buf.String())

	buf.Reset()

	b.AddSection(
		"/etc/prometheus/node_alerts.yaml",
		"node-exporter",
		"NodeFilesystemSpaceFillingUp",
		`(node_filesystem_avail_bytes{fstype!="",job="node"} / node_filesystem_size_bytes{fstype!="",job="node"} * 100 < 40 and predict_linear(node_filesystem_avail_bytes{fstype!="",job="node"}[6h], 24 * 60 * 60) < 0 and node_filesystem_readonly{fstype!="",job="node"} == 0)`,
		[]string{},
		[]string{
			`node_filesystem_avail_bytes{fstype!="",job="node"}`,
			`node_filesystem_size_bytes{fstype!="",job="node"}`,
			`node_filesystem_avail_bytes{fstype!="",job="node"}`,
			`node_filesystem_readonly{fstype!="",job="node"}`,
		},
	)
	expected := `
{
  "promcheck": {
    "results": [
      {
        "file": "/etc/prometheus/node_alerts.yaml",
        "group": "node-exporter",
        "name": "NodeFilesystemSpaceFillingUp",
        "expression": "(node_filesystem_avail_bytes{fstype!=\"\",job=\"node\"} / node_filesystem_size_bytes{fstype!=\"\",job=\"node\"} * 100 \u003c 40 and predict_linear(node_filesystem_avail_bytes{fstype!=\"\",job=\"node\"}[6h], 24 * 60 * 60) \u003c 0 and node_filesystem_readonly{fstype!=\"\",job=\"node\"} == 0)",
        "no_results": [],
        "results": [
          "node_filesystem_avail_bytes{fstype!=\"\",job=\"node\"}",
          "node_filesystem_size_bytes{fstype!=\"\",job=\"node\"}",
          "node_filesystem_avail_bytes{fstype!=\"\",job=\"node\"}",
          "node_filesystem_readonly{fstype!=\"\",job=\"node\"}"
        ]
      }
    ],
    "rules_warnings": 1,
    "selectors_success_total": 4
  }
}`
	require.NoError(t, b.DumpJSON())
	require.JSONEq(t, expected, buf.String())
}

func TestBuilder_DumpTree(t *testing.T) {
	buf := &bytes.Buffer{}
	b := NewBuilder(WithWriter(buf), WithoutColor())

	expected := `
.

Groups total: 0, Rules total: 0
Selectors total: 0, Results found: 0, No Results found 0 (No Results/Total: NaN%)
`
	require.NoError(t, b.DumpTree())
	require.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(buf.String()))

	buf.Reset() // clear the output buffer for the next test

	b.AddSection(
		"/etc/prometheus/node_alerts.yaml",
		"node-exporter",
		"NodeFilesystemSpaceFillingUp",
		`(node_filesystem_avail_bytes{fstype!="",job="node"} / node_filesystem_size_bytes{fstype!="",job="node"} * 100 < 40 and predict_linear(node_filesystem_avail_bytes{fstype!="",job="node"}[6h], 24 * 60 * 60) < 0 and node_filesystem_readonly{fstype!="",job="node"} == 0)`,
		[]string{
			`node_filesystem_avail_bytes{fstype!="",job="node"}`,
		},
		[]string{
			`node_filesystem_size_bytes{fstype!="",job="node"}`,
			`node_filesystem_avail_bytes{fstype!="",job="node"}`,
			`node_filesystem_readonly{fstype!="",job="node"}`,
		},
	)

	expected = `
.
└── [file] /etc/prometheus/node_alerts.yaml
    └── [group] node-exporter
        └── [3/4] NodeFilesystemSpaceFillingUp
            ├── [✔] node_filesystem_size_bytes{fstype!="",job="node"}
            ├── [✔] node_filesystem_avail_bytes{fstype!="",job="node"}
            ├── [✔] node_filesystem_readonly{fstype!="",job="node"}
            └── [✖] node_filesystem_avail_bytes{fstype!="",job="node"}

Groups total: 0, Rules total: 0
Selectors total: 4, Results found: 3, No Results found 1 (No Results/Total: 25.00%)
`

	require.NoError(t, b.DumpTree())
	require.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(buf.String()))
}
