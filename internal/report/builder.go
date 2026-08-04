package report

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/mattn/go-isatty"
	"gopkg.in/yaml.v3"

	"github.com/cbrgm/promcheck/internal/metrics"
)

const (
	// DefaultFormat dumps Report as Text.
	DefaultFormat = "graph"

	// YAMLFormat dumps Report as YAML.
	YAMLFormat = "yaml"

	// JSONFormat dumps Report as JSON.
	JSONFormat = "json"

	// PrometheusFormat converts Report to Prometheus metrics.
	// This format is only used internally by Promcheck and cannot be set via cli flags.
	PrometheusFormat = "prometheus"
)

// Builder represents the report.
type Builder struct {
	// Report represents the report data
	Report Report `json:"promcheck" yaml:"promcheck"`

	// format represents the output format
	format string

	// writer represents the output target
	writer io.Writer

	// metrics represents promcheck metrics
	metrics metrics.Metrics

	// useColor controls whether the tree renderer emits ANSI color codes.
	useColor bool

	// colorExplicit is true once WithColor/WithoutColor has been applied,
	// so NewBuilder knows not to overwrite it with its own auto-detection.
	colorExplicit bool

	// onlyFailing restricts rendered sections (tree/json/yaml) to ones with
	// at least one selector without a result. Summary totals are unaffected.
	onlyFailing bool
}

// NewBuilder returns a new Builder.
func NewBuilder(opts ...BuilderOption) *Builder {
	b := &Builder{
		Report:  Report{Sections: Sections{}},
		format:  DefaultFormat,
		writer:  os.Stdout,
		metrics: metrics.NewDefaultPrometheus(),
	}
	for _, opt := range opts {
		opt(b)
	}
	if !b.colorExplicit {
		// Default: color only when writing to a real terminal and the user
		// hasn't opted out via NO_COLOR (see no-color.org).
		b.useColor = IsTTY(b.writer) && os.Getenv("NO_COLOR") == ""
	}
	return b
}

// IsTTY reports whether w is a terminal file descriptor. Writers that aren't
// an *os.File (e.g. a bytes.Buffer used in tests, or a pipe) are never
// considered a TTY.
func IsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// BuilderOption represents builder options.
type BuilderOption func(*Builder)

// WithFormat sets the builder's output format.
func WithFormat(format string) BuilderOption {
	return func(b *Builder) {
		b.format = format
	}
}

// WithoutColor passing this BuilderOption to the NewBuilder disables terminal color.
func WithoutColor() BuilderOption {
	return WithColor(false)
}

// WithColor explicitly enables or disables ANSI color codes in tree output,
// overriding the Builder's own TTY/NO_COLOR auto-detection.
func WithColor(enabled bool) BuilderOption {
	return func(b *Builder) {
		b.useColor = enabled
		b.colorExplicit = true
	}
}

// WithOnlyFailing restricts rendered output (tree, json, yaml) to sections
// that have at least one selector without a result. Summary totals (groups,
// rules, selectors, ratio) continue to reflect the full run.
func WithOnlyFailing() BuilderOption {
	return func(b *Builder) {
		b.onlyFailing = true
	}
}

// WithWriter specifies a custom io.Writer to write to.
// By default, os.Stdout is used.
func WithWriter(w io.Writer) BuilderOption {
	return func(b *Builder) {
		b.writer = w
	}
}

// WithMetrics configures the Builder to use custom specified metrics.
func WithMetrics(metrics metrics.Metrics) BuilderOption {
	return func(b *Builder) {
		b.metrics = metrics
	}
}

// Report represents report data.
type Report struct {
	// Sections represents a list of result data
	Sections Sections `json:"results" yaml:"results"`

	// TotalRules represents the total amount of checked groups
	TotalGroups int `json:"groups_total" yaml:"groups_total"`

	// TotalGroups represents the total amount of checked rules
	TotalRules int `json:"rules_total" yaml:"rules_total"`

	// TotalSelectorsFailed represents the total amount of probed selectors not containing a result value
	TotalSelectorsFailed int `json:"selectors_failed_total" yaml:"selectors_failed_total"`

	// TotalSelectorsSuccess represents the total amount of probed selectors containing a result value
	TotalSelectorsSuccess int `json:"selectors_success_total" yaml:"selectors_success_total"`

	// RatioFailedTotal represents the ratio of selectors without a result value / total amount of selectors
	RatioFailedTotal float32 `json:"ratio_failed_total" yaml:"ratio_failed_total"`
}

// Sections represents a collection of sections.
type Sections []Section

// Section represents a report section.
type Section struct {
	// File represents the file name of the checked rule
	File string `json:"file" yaml:"file"`

	// Group represents the group name of the checked rule
	Group string `json:"group" yaml:"group"`

	// Name represents the recording rule or alert name
	Name string `json:"name" yaml:"name"`

	// Expression represents the rule's PromQL expression string
	Expression string `json:"expression" yaml:"expression"`

	// NoResults represents a list of the rule's PromQL selectors which did not successfully returned a result value
	NoResults []string `json:"no_results" yaml:"no_results"`

	// Results represents a list of the rule's PromQL selectors which successfully returned a result value
	Results []string `json:"results" yaml:"results"`
}

// Len returns the list size.
func (s Report) Len() int {
	return len(s.Sections)
}

// HasContent checks if we actually have anything to report.
func (b *Builder) HasContent() bool {
	return b.Report.TotalRules != 0
}

// finalize is called by format functions and calculates additional report data.
func (b *Builder) finalize() {
	// Sections are appended by concurrent probes upstream, so their order is
	// non-deterministic across runs. Sort them so every output format (json,
	// yaml, tree) renders the same way given the same input, which keeps CI
	// diffs sane. Expression is included as a final tiebreaker: two sections
	// can legally share the same (File, Group, Name) — e.g. two alerts named
	// "HighLatency" in the same group at different severities — and without
	// a total sort key, their relative order would depend on the
	// non-deterministic pre-sort order.
	slices.SortFunc(b.Report.Sections, func(a, c Section) int {
		return cmp.Or(
			cmp.Compare(a.File, c.File),
			cmp.Compare(a.Group, c.Group),
			cmp.Compare(a.Name, c.Name),
			cmp.Compare(a.Expression, c.Expression),
		)
	})

	totalSelectors := b.Report.TotalSelectorsFailed + b.Report.TotalSelectorsSuccess
	if totalSelectors == 0 {
		b.Report.RatioFailedTotal = 0
		return
	}
	b.Report.RatioFailedTotal = (float32(b.Report.TotalSelectorsFailed) / float32(totalSelectors)) * 100
}

// clear resets the report.
func (b *Builder) clear() {
	b.Report = Report{Sections: Sections{}}
}

// AddSection adds a new section to the report.
func (b *Builder) AddSection(file, group, name, expression string, failed, success []string) {
	b.Report.Sections = append(b.Report.Sections, Section{
		File:       file,
		Group:      group,
		Name:       name,
		Expression: expression,
		NoResults:  failed,
		Results:    success,
	})

	b.Report.TotalRules++
	b.Report.TotalSelectorsFailed += len(failed)
	b.Report.TotalSelectorsSuccess += len(success)
}

// AddTotalCheckedGroups adds checked groups to the total amount.
// TotalGroups is used for report metrics.
func (b *Builder) AddTotalCheckedGroups(count int) {
	b.Report.TotalGroups += count
}

// reportEnvelope mirrors the shape Builder marshals to json/yaml (a single
// "promcheck" key), decoupled from Builder itself so rendering can swap in a
// filtered Report (see renderedReport) without mutating the Builder's state.
type reportEnvelope struct {
	Report Report `json:"promcheck" yaml:"promcheck"`
}

// renderedReport returns the Report to marshal/print. When onlyFailing is
// set, sections without a failing selector are dropped from the copy, but
// the summary totals (which are accumulated independently in AddSection)
// still reflect every section from the full run.
func (b *Builder) renderedReport() Report {
	r := b.Report
	if !b.onlyFailing {
		return r
	}
	filtered := make(Sections, 0, len(r.Sections))
	for _, s := range r.Sections {
		if len(s.NoResults) > 0 {
			filtered = append(filtered, s)
		}
	}
	r.Sections = filtered
	return r
}

// ToYAML returns the report in yaml format.
func (b *Builder) ToYAML() (string, error) {
	b.finalize()
	raw, err := yaml.Marshal(reportEnvelope{Report: b.renderedReport()})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// ToJSON returns the report in json format.
func (b *Builder) ToJSON() (string, error) {
	b.finalize()
	raw, err := json.MarshalIndent(reportEnvelope{Report: b.renderedReport()}, "", "  ")
	if err != nil {
		return "", err
	}

	return string(raw), nil
}

// Dump prints the report to the builder's output target in the desired format.
func (b *Builder) Dump() error {
	if !b.HasContent() {
		return errors.New("nothing to report")
	}
	var err error
	switch b.format {
	case YAMLFormat:
		err = b.DumpYAML()
	case JSONFormat:
		err = b.DumpJSON()
	case PrometheusFormat:
		err = b.DumpPrometheusMetrics()
	default:
		err = b.DumpTree()
	}
	return err
}

// DumpYAML prints the report to the builder's output target in yaml format.
func (b *Builder) DumpYAML() error {
	defer b.clear()
	res, err := b.ToYAML()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(b.writer, "%v\n", res)
	return nil
}

// DumpJSON prints the report to the builder's output target in json format.
func (b *Builder) DumpJSON() error {
	defer b.clear()
	res, err := b.ToJSON()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(b.writer, "%v\n", res)
	return nil
}

// DumpTree prints the report to the builder's output target in text format.
func (b *Builder) DumpTree() error {
	defer b.clear()
	res, err := b.ToTree()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(b.writer, "%v\n", res)
	return nil
}

// DumpPrometheusMetrics converts the report to Prometheus metrics.
func (b *Builder) DumpPrometheusMetrics() error {
	defer b.clear()
	err := b.ToPrometheusMetrics()
	if err != nil {
		return err
	}
	return nil
}
