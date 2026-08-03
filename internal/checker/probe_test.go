package checker

import (
	"context"
	"testing"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

type fakeAPI struct {
	prometheusv1.API // embed so unimplemented methods panic if called
	value            model.Value
	warnings         prometheusv1.Warnings
	err              error

	// gotTS records the evaluation timestamp passed to the last Query call
	gotTS time.Time
}

func (f *fakeAPI) Query(_ context.Context, _ string, ts time.Time, _ ...prometheusv1.Option) (model.Value, prometheusv1.Warnings, error) {
	f.gotTS = ts
	return f.value, f.warnings, f.err
}

func TestProbe_NonVectorResultReturnsError(t *testing.T) {
	// A scalar result must not panic; it must return an error.
	p := &prometheusProbe{api: &fakeAPI{value: &model.Scalar{Value: 1, Timestamp: 0}}}
	_, err := p.probe(context.Background(), `up`, time.Now())
	require.Error(t, err)
}

func TestProbe_VectorResultReturnsCount(t *testing.T) {
	vec := model.Vector{&model.Sample{Value: 3}}
	p := &prometheusProbe{api: &fakeAPI{value: vec}}
	v, err := p.probe(context.Background(), `up`, time.Now())
	require.NoError(t, err)
	require.Equal(t, float64(3), v)
}

func TestProbe_UsesGivenTimestamp(t *testing.T) {
	vec := model.Vector{&model.Sample{Value: 1}}
	api := &fakeAPI{value: vec}
	p := &prometheusProbe{api: api}
	ts := time.Now().Add(-5 * time.Minute)
	_, err := p.probe(context.Background(), `up`, ts)
	require.NoError(t, err)
	require.True(t, api.gotTS.Equal(ts), "probe must query at the given ts, got %v want %v", api.gotTS, ts)
}
