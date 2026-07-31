package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"time"

	"github.com/alecthomas/kong"
)

const (
	levelDebug = "debug"
	levelInfo  = "info"
	levelWarn  = "warn"
	levelError = "error"
)

// Exit codes. This is the documented promcheck CLI contract: scripts and CI
// pipelines may depend on these values, so they must not change casually.
const (
	// exitOK means the run completed with no findings (or wasn't strict).
	exitOK = 0
	// exitFindings means --strict was set and one or more selectors had no results.
	exitFindings = 1
	// exitUsage means a usage or configuration error: bad flags, a bad
	// regexp, or nothing matched to check.
	exitUsage = 2
	// exitRuntime means a runtime failure while probing: connection,
	// query, or parse error.
	exitRuntime = 3
)

var (
	// Version of promcheck.
	Version string
	// Revision or Commit this binary was built from.
	Revision string
	// GoVersion running this binary.
	GoVersion = runtime.Version()
	// StartTime has the time this was started.
	StartTime = time.Now()
)

type config struct {
	// PrometheusURL represents the URL prometheus is running at. Required.
	PrometheusURL               string `required:"true" name:"prometheus.url" default:"http://0.0.0.0:9090" help:"The Prometheus base url"`
	PrometheusBasicAuthUsername string `name:"prometheus.basic-auth-user" default:"" help:"Basic auth username"`
	PrometheusBasicAuthPassword string `name:"prometheus.basic-auth-pass" default:"" help:"Basic auth password"`

	// check parameters
	CheckIgnoredSelectorsRegexp []string `name:"check.ignore-selector" help:"Regexp of selectors to ignore"`
	CheckIgnoredGroupsRegexp    []string `name:"check.ignore-group" help:"Regexp of rule groups to ignore"`
	CheckConcurrency            int      `name:"check.concurrency" default:"8" help:"Maximum number of selectors probed in parallel"`
	CheckFiles                  string   `name:"check.file" help:"The rule files to check."`
	CheckExpressions            []string `name:"check.query" help:"Inline PromQL expression to check"`
	CheckMatch                  []string `name:"check.match" help:"PromQL label matchers to filter rules server-side, e.g. '{team=\"infra\"}'"`

	// output parameters
	OutputFormat      string `name:"output.format" enum:"graph,json,yaml" default:"graph" help:"The output format to use"`
	OutputNoColor     bool   `name:"output.no-color" default:"false" help:"Toggle colored output"`
	OutputOnlyFailing bool   `name:"output.only-failing" default:"false" help:"Only show rules that have selectors without results"`

	// exporter parameters
	ExporterModeEnabled          bool   `name:"exporter.enabled" default:"false" help:"Run promcheck as a prometheus exporter"`
	ExporterHTTPAddr             string `name:"exporter.addr" default:"0.0.0.0:9093" help:"The address the http server is running at"`
	ExporterInterval             int    `name:"exporter.interval" default:"300" help:"Delay in seconds between promcheck runs"`
	ExporterEnableProfiling      bool   `name:"metrics.profile" default:"true" help:"Enable pprof profiling"`
	ExporterEnableRuntimeMetrics bool   `name:"metrics.runtime" default:"true" help:"Enable runtime metrics"`
	ExporterMetricsPrefix        string `name:"metrics.prefix" default:"" help:"Set metrics prefix path"`

	// log parameters
	LogJSON  bool   `name:"log.json" default:"false" help:"Tell promcheck to log json and not key value pairs"`
	LogLevel string `name:"log.level" default:"info" enum:"error,warn,info,debug" help:"The log level to use for filtering logs"`

	// etc
	StrictMode bool `name:"strict" default:"false" help:"Tell promcheck to exit with an error code on expressions without results"`
}

func main() {
	cfg := config{}
	_ = kong.Parse(&cfg,
		kong.Name("promcheck"),
		kong.Description(
			fmt.Sprintf(
				"A tool to identify faulty Prometheus rules\n Version: %s %s\n BuildTime: %s\n %s\n",
				Revision,
				Version,
				StartTime.Format("2006-01-02"),
				GoVersion,
			),
		),
	)

	logger := newLogger(cfg.LogJSON, cfg.LogLevel)

	os.Exit(runMain(&cfg, logger))
}

// runMain wires up and executes promcheck, returning the process exit code.
// It exists separately from main so the exit-code contract can be exercised
// without an os.Exit call terminating the test process.
func runMain(cfg *config, logger *slog.Logger) int {
	// validation
	if cfg.ExporterInterval < 0 {
		logger.Error("configuration error", "err", "--exporter.interval must be > 0")
		return exitUsage
	}

	if cfg.CheckConcurrency < 1 {
		logger.Error("configuration error", "err", "--check.concurrency must be >= 1")
		return exitUsage
	}

	// initialize promcheck
	app, err := newPromcheck(cfg, logger)
	if err != nil {
		return exitUsage
	}

	err = app.run()
	code := exitCodeFor(err)
	if code == exitRuntime {
		logger.Error("promcheck failed", "err", err)
	}
	return code
}

// exitCodeFor maps a promcheckApp.run error to the documented exit-code
// contract (see the exit* constants above).
func exitCodeFor(err error) int {
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, ErrStrictFindings):
		return exitFindings
	case errors.Is(err, ErrNoRuleGroups):
		return exitUsage
	default:
		return exitRuntime
	}
}

func newLogger(jsonOut bool, lvl string) *slog.Logger {
	var level slog.Level
	switch lvl {
	case levelDebug:
		level = slog.LevelDebug
	case levelWarn:
		level = slog.LevelWarn
	case levelError:
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler = slog.NewTextHandler(os.Stderr, opts)
	if jsonOut {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	return slog.New(h)
}
