package metrics

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

const defaultMetricsPath = "/metrics"

// Options for initializing metrics collection.
type Options struct {
	// Common prefix for the keys of the different
	// collected metrics.
	Prefix string

	// EnableProfile exposes profiling information on /pprof of the
	// metrics listener.
	EnableProfile bool

	// enables go runtime metrics
	EnableRuntimeMetrics bool

	// A new registry is created if this option is nil.
	PrometheusRegistry *prometheus.Registry
}

func DefaultOptions() Options {
	return Options{
		Prefix:               "",
		EnableProfile:        false,
		EnableRuntimeMetrics: true,
		PrometheusRegistry:   nil,
	}
}

// Metrics is the generic interface that all the required backends
// should implement to be a promcheck metrics compatible backend.
type Metrics interface {
	SetRuleGroupsTotal(value float64)
	SetRulesTotal(value float64)
	SetSelectorsTotal(file, group, rule, status string, value float64)
	RegisterHandler(path string, handler *http.ServeMux)
}

// NewDefaultHandler returns a default metrics handler.
func NewDefaultHandler(o Options) http.Handler {
	m := NewPrometheus(o)
	return HandlerFor(m, o)
}

// HandlerFor returns a collection of metrics handlers.
func HandlerFor(m Metrics, o Options) http.Handler {
	mux := http.NewServeMux()
	if o.EnableProfile {
		mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
		mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	}

	// Root path should return 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Fix trailing slashes and register routes.
	mPath := defaultMetricsPath
	mPath = strings.TrimRight(mPath, "/")
	m.RegisterHandler(mPath, mux)
	mPath = fmt.Sprintf("%s/", mPath)
	m.RegisterHandler(mPath, mux)

	return mux
}
