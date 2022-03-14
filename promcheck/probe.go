package promcheck

import (
	"context"
	"fmt"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"time"
)

// Prober represents probe
type Prober interface {
	// ProbeSelector probes the given PromQL selector against a remote instance.
	ProbeSelector(selector string) (float64, error)
}

type prometheusProbe struct {
	api           prometheusv1.API
	delay         time.Duration
	prometheusUrl string
}

func newPrometheusProbe(delay time.Duration, prometheusUrl string, client prometheusv1.API) Prober {
	return &prometheusProbe{
		api:           client,
		delay:         delay,
		prometheusUrl: prometheusUrl,
	}
}

func (p *prometheusProbe) probe(selector string) (float64, error) {
	query := fmt.Sprintf("count(%s)", selector)
	value, _, err := p.api.Query(context.TODO(), query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to query metrics: %w", err)
	}
	vec := value.(model.Vector)
	var metricValue float64
	for _, v := range vec {
		if v.Value.String() == "NaN" {
			metricValue = 0
		} else {
			metricValue = float64(v.Value)
		}
	}
	return metricValue, nil
}

// ProbeSelector implements Prober
func (p *prometheusProbe) ProbeSelector(selector string) (float64, error) {
	v, err := p.probe(selector)
	if err != nil {
		return 0, err
	}
	time.Sleep(p.delay)
	return v, nil
}
