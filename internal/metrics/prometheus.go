package metrics

import (
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	promNamespace       = "promcheck"
	promChecksSubsystem = "validation"
)

// Prometheus implements the prometheus metrics backend.
type Prometheus struct {
	ruleGroupsGaugeM *prometheus.GaugeVec
	rulesGaugeM      *prometheus.GaugeVec
	selectorsGaugeM  *prometheus.GaugeVec

	buildInfoGaugeM   *prometheus.GaugeVec
	lastRunTimestampM prometheus.Gauge
	runDurationM      prometheus.Gauge
	runErrorsTotalM   prometheus.Counter

	opts     Options
	registry *prometheus.Registry
	handler  http.Handler
}

func NewDefaultPrometheus() *Prometheus {
	return NewPrometheus(DefaultOptions())
}

// NewPrometheus returns a new Prometheus metric backend.
func NewPrometheus(opts Options) *Prometheus {
	namespace := promNamespace
	if opts.Prefix != "" {
		namespace = strings.TrimSuffix(opts.Prefix, ".")
	}

	ruleGroupsTotal := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: promChecksSubsystem,
		Name:      "rule_groups_total",
		Help:      "Total number of evaluated rule groups.",
	}, []string{})

	rulesTotal := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: promChecksSubsystem,
		Name:      "rules_total",
		Help:      "Total number of evaluated rules.",
	}, []string{})

	selectorsTotal := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: promChecksSubsystem,
		Name:      "selectors_total",
		Help:      "Total number of evaluated selectors.",
	}, []string{"file", "group", "rule", "status"})

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "build_info",
		Help:      "Build information about promcheck, value is always 1.",
	}, []string{"version", "revision", "goversion"})

	lastRunTimestamp := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "last_run_timestamp_seconds",
		Help:      "Unix timestamp of the last check run.",
	})

	runDuration := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "run_duration_seconds",
		Help:      "Duration of the last check run in seconds.",
	})

	runErrorsTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "run_errors_total",
		Help:      "Total number of check cycles that returned an error.",
	})

	p := &Prometheus{
		ruleGroupsGaugeM:  ruleGroupsTotal,
		rulesGaugeM:       rulesTotal,
		selectorsGaugeM:   selectorsTotal,
		buildInfoGaugeM:   buildInfo,
		lastRunTimestampM: lastRunTimestamp,
		runDurationM:      runDuration,
		runErrorsTotalM:   runErrorsTotal,
		opts:              opts,
		registry:          opts.PrometheusRegistry,
		handler:           nil,
	}

	if p.registry == nil {
		p.registry = prometheus.NewRegistry()
	}
	p.registerMetrics()
	return p
}

func (p *Prometheus) registerMetrics() {
	p.registry.MustRegister(p.ruleGroupsGaugeM)
	p.registry.MustRegister(p.rulesGaugeM)
	p.registry.MustRegister(p.selectorsGaugeM)
	p.registry.MustRegister(p.buildInfoGaugeM)
	p.registry.MustRegister(p.lastRunTimestampM)
	p.registry.MustRegister(p.runDurationM)
	p.registry.MustRegister(p.runErrorsTotalM)

	if p.opts.EnableRuntimeMetrics {
		p.registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
		p.registry.MustRegister(collectors.NewGoCollector())
	}
}

func (p *Prometheus) CreateHandler() http.Handler {
	return promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{})
}

func (p *Prometheus) getHandler() http.Handler {
	if p.handler != nil {
		return p.handler
	}
	p.handler = p.CreateHandler()
	return p.handler
}

// RegisterHandler satisfies Metrics interface.
func (p *Prometheus) RegisterHandler(path string, mux *http.ServeMux) {
	promHandler := p.getHandler()
	mux.Handle(path, promHandler)
}

func (p *Prometheus) SetRuleGroupsTotal(value float64) {
	p.ruleGroupsGaugeM.WithLabelValues().Set(value)
}

func (p *Prometheus) SetRulesTotal(value float64) {
	p.rulesGaugeM.WithLabelValues().Set(value)
}

func (p *Prometheus) SetSelectorsTotal(file, group, rule, status string, value float64) {
	p.selectorsGaugeM.WithLabelValues(file, group, rule, status).Set(value)
}

func (p *Prometheus) SetBuildInfo(version, revision, goversion string) {
	p.buildInfoGaugeM.WithLabelValues(version, revision, goversion).Set(1)
}

func (p *Prometheus) SetLastRunTimestamp(t time.Time) {
	p.lastRunTimestampM.Set(float64(t.Unix()))
}

func (p *Prometheus) SetRunDuration(d time.Duration) {
	p.runDurationM.Set(d.Seconds())
}

func (p *Prometheus) IncRunErrors() {
	p.runErrorsTotalM.Inc()
}
