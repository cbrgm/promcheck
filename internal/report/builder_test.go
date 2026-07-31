package report

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
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
    "groups_total": 0,
    "rules_total": 1,
    "selectors_failed_total": 0,
    "selectors_success_total": 4,
    "ratio_failed_total": 0
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
Selectors total: 0, Results found: 0, No Results found 0 (No Results/Total: 0.00%)
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

Groups total: 0, Rules total: 1
Selectors total: 4, Results found: 3, No Results found 1 (No Results/Total: 25.00%)
`

	require.NoError(t, b.DumpTree())
	require.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(buf.String()))
}

func TestReport_SummaryFieldsAlwaysPresent(t *testing.T) {
	b := NewBuilder(WithWriter(io.Discard))
	b.AddSection("f", "g", "r", "vector(1)", nil, []string{"a", "b"})

	raw, err := b.ToJSON()
	require.NoError(t, err)

	var decoded map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
	promcheck := decoded["promcheck"]

	for _, key := range []string{
		"results",
		"groups_total",
		"rules_total",
		"selectors_failed_total",
		"selectors_success_total",
		"ratio_failed_total",
	} {
		require.Contains(t, promcheck, key, "expected key %q to always be present", key)
	}
	require.Equal(t, float64(0), promcheck["selectors_failed_total"])
	require.Equal(t, float64(0), promcheck["groups_total"])
}

func TestReport_EmptySectionsMarshalAsEmptyArray(t *testing.T) {
	b := NewBuilder(WithWriter(io.Discard))

	raw, err := b.ToJSON()
	require.NoError(t, err)
	require.Contains(t, raw, `"results": []`)
	require.NotContains(t, raw, `"results": null`)

	yamlRaw, err := b.ToYAML()
	require.NoError(t, err)
	require.Contains(t, yamlRaw, "results: []")
}

func TestFinalize_SortsSectionsDeterministically(t *testing.T) {
	newScrambled := func() *Builder {
		b := NewBuilder(WithWriter(io.Discard))
		b.AddSection("c-file", "g", "z-rule", "vector(1)", nil, nil)
		b.AddSection("a-file", "z-group", "r", "vector(1)", nil, nil)
		b.AddSection("a-file", "a-group", "b-rule", "vector(1)", nil, nil)
		b.AddSection("a-file", "a-group", "a-rule", "vector(1)", nil, nil)
		b.AddSection("b-file", "g", "r", "vector(1)", nil, nil)
		return b
	}

	b1 := newScrambled()
	tree1, err := b1.ToTree()
	require.NoError(t, err)

	b2 := newScrambled()
	tree2, err := b2.ToTree()
	require.NoError(t, err)

	require.Equal(t, tree1, tree2, "rendering the same scrambled input twice must produce identical output")

	want := []struct{ file, group, name string }{
		{"a-file", "a-group", "a-rule"},
		{"a-file", "a-group", "b-rule"},
		{"a-file", "z-group", "r"},
		{"b-file", "g", "r"},
		{"c-file", "g", "z-rule"},
	}
	require.Len(t, b1.Report.Sections, len(want))
	for i, w := range want {
		require.Equal(t, w.file, b1.Report.Sections[i].File, "section %d file", i)
		require.Equal(t, w.group, b1.Report.Sections[i].Group, "section %d group", i)
		require.Equal(t, w.name, b1.Report.Sections[i].Name, "section %d name", i)
	}
}

// ansiEscape is present in fatih/color output whenever colorization is active.
const ansiEscape = "\x1b["

func newColorTestBuilder(opts ...BuilderOption) (*Builder, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	b := NewBuilder(append([]BuilderOption{WithWriter(buf)}, opts...)...)
	b.AddSection(
		"f.yaml", "g", "r", "vector(1)",
		[]string{"failed_selector"},
		[]string{"ok_selector"},
	)
	return b, buf
}

func TestToTree_ColorForcedOff_NoANSICodes(t *testing.T) {
	b, _ := newColorTestBuilder(WithoutColor())
	out, err := b.ToTree()
	require.NoError(t, err)
	require.NotContains(t, out, ansiEscape)
}

func TestToTree_NonTTYWriter_DefaultsToNoColor(t *testing.T) {
	// bytes.Buffer is never a TTY, so the default (no explicit color option)
	// must resolve to colorless output, matching NO_COLOR/non-terminal
	// behavior without a global fatih/color mutation.
	b, _ := newColorTestBuilder()
	out, err := b.ToTree()
	require.NoError(t, err)
	require.NotContains(t, out, ansiEscape)
}

func TestToTree_ColorForcedOn_EmitsANSICodes(t *testing.T) {
	b, _ := newColorTestBuilder(WithColor(true))
	out, err := b.ToTree()
	require.NoError(t, err)
	require.Contains(t, out, ansiEscape)
}

func TestFinalize_ZeroSelectorsNoNaN(t *testing.T) {
	b := NewBuilder(WithWriter(io.Discard))
	// A rule with no selectors: TotalRules > 0, but total selectors == 0.
	b.AddSection("f", "g", "r", "vector(1)", nil, nil)
	b.finalize()
	require.False(t, math.IsNaN(float64(b.Report.RatioFailedTotal)))
	require.Equal(t, float32(0), b.Report.RatioFailedTotal)
}
