package promcheck

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
}

func (f *fakeAPI) Query(_ context.Context, _ string, _ time.Time, _ ...prometheusv1.Option) (model.Value, prometheusv1.Warnings, error) {
	return f.value, f.warnings, f.err
}

func TestProbe_NonVectorResultReturnsError(t *testing.T) {
	// A scalar result must not panic; it must return an error.
	p := &prometheusProbe{api: &fakeAPI{value: &model.Scalar{Value: 1, Timestamp: 0}}}
	_, err := p.probe(`up`)
	require.Error(t, err)
}

func TestProbe_VectorResultReturnsCount(t *testing.T) {
	vec := model.Vector{&model.Sample{Value: 3}}
	p := &prometheusProbe{api: &fakeAPI{value: vec}}
	v, err := p.probe(`up`)
	require.NoError(t, err)
	require.Equal(t, float64(3), v)
}
