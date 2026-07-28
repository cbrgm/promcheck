package promcheck

import (
	"context"
	"fmt"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// Prober represents probe.
type Prober interface {
	// ProbeSelector probes the given PromQL selector against a remote instance.
	ProbeSelector(ctx context.Context, selector string) (float64, error)
}

type prometheusProbe struct {
	api           prometheusv1.API
	prometheusURL string
}

func newPrometheusProbe(prometheusURL string, client prometheusv1.API) Prober {
	return &prometheusProbe{
		api:           client,
		prometheusURL: prometheusURL,
	}
}

func (p *prometheusProbe) probe(ctx context.Context, selector string) (float64, error) {
	query := fmt.Sprintf("count(%s)", selector)
	value, _, err := p.api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to query metrics: %w", err)
	}
	vec, ok := value.(model.Vector)
	if !ok {
		return 0, fmt.Errorf("unexpected query result type %T for selector %q (wanted vector)", value, selector)
	}
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

// ProbeSelector implements Prober.
func (p *prometheusProbe) ProbeSelector(ctx context.Context, selector string) (float64, error) {
	return p.probe(ctx, selector)
}
