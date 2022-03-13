package main

import (
	"fmt"
	"github.com/alecthomas/kong"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"os"
	"runtime"
	"time"
)

const (
	levelDebug = "debug"
	levelInfo  = "info"
	levelWarn  = "warn"
	levelError = "error"
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

	// PrometheusUrl represents the URL prometheus is running at. Required.
	PrometheusUrl string `required:"true" name:"prometheus.url" default:"http://0.0.0.0:9090" help:"The Prometheus base url"`

	// check parameters
	CheckIgnoredSelectorsRegexp []string `name:"check.ignore-selector" help:"Regexp of selectors to ignore"`
	CheckIgnoredGroupsRegexp    []string `name:"check.ignore-group" help:"Regexp of rule groups to ignore"`
	CheckDelay                  float64  `name:"check.delay" default:"0.1" help:"Delay in seconds between probe requests"`
	CheckFiles                  string   `required:"true" name:"check.file" help:"The rule files to check."`

	// output parameters
	OutputFormat  string `name:"output.format" enum:"graph,json,yaml,csv" default:"graph" help:"The output format to use"`
	OutputNoColor bool   `name:"output.no-color" default:"false" help:"Toggle colored output"`

	// log parameters
	LogJSON  bool   `name:"log.json" default:"false" help:"Tell promcheck to log json and not key value pairs"`
	LogLevel string `name:"log.level" default:"info" enum:"error,warn,info,debug" help:"The log level to use for filtering logs"`
}

func main() {
	config := config{}
	_ = kong.Parse(&config,
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

	levelFilter := map[string]level.Option{
		levelError: level.AllowError(),
		levelWarn:  level.AllowWarn(),
		levelInfo:  level.AllowInfo(),
		levelDebug: level.AllowDebug(),
	}

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	if config.LogJSON {
		logger = log.NewJSONLogger(log.NewSyncWriter(os.Stderr))
	}

	logger = level.NewFilter(logger, levelFilter[config.LogLevel])
	logger = log.With(logger,
		"ts", log.DefaultTimestampUTC,
		"caller", log.DefaultCaller,
	)

	err := checkRulesFromRuleFiles(&config, logger)
	if err != nil {
		level.Error(logger).Log("msg", "failed to check rule files", "err", err)
		os.Exit(1)
	}
	os.Exit(0)
}
