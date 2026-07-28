package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oklog/run"

	"github.com/cbrgm/promcheck/promcheck/metrics"
)

func (app *promcheckApp) runPromcheckExporter() error {
	ctx, cancel := context.WithCancel(context.Background())
	var gr run.Group
	// http server
	{
		httpLogger := app.logger.With("component", "exporter")
		m := http.NewServeMux()
		handleHealth := func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
		m.HandleFunc("/health", handleHealth)
		m.HandleFunc("/healthz", handleHealth)
		m.Handle("/metrics", metrics.HandlerFor(app.metrics, metrics.Options{
			Prefix:               app.optExporterMetricsPrefix,
			EnableProfile:        app.optExporterEnableProfiling,
			EnableRuntimeMetrics: app.optExporterEnableRuntimeMetrics,
			PrometheusRegistry:   nil,
		}))

		s := http.Server{
			Addr:    app.optExporterHTTPAddr,
			Handler: m,
		}

		m.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html>
			<head><title>Promcheck Exporter</title></head>
			<body>
			<h1>promcheckApp Exporter</h1>
			<p><a href="` + app.optExporterMetricsPrefix + `">see metrics</a></p>
			</body>
			</html>`))
		})
		gr.Add(func() error {
			httpLogger.Info("running http server", "addr", s.Addr)

			return s.ListenAndServe()
		}, func(_ error) {
			_ = s.Shutdown(context.TODO())
		})
	}
	// promcheck
	{
		tick := time.NewTicker(app.optExporterInterval)
		defer tick.Stop()
		gr.Add(func() error {
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-tick.C:
					app.logger.Info("executing promcheck routine")
					if err := app.checkRules(); err != nil {
						return err
					}
				}
			}
		}, func(err error) {
			app.logger.Info("error while executing promcheck routine", "err", err)
		})
	}
	{
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		gr.Add(func() error {
			<-sig
			return nil
		}, func(_ error) {
			cancel()
			close(sig)
		})
	}

	if err := gr.Run(); err != nil {
		return fmt.Errorf("error running: %w", err)
	}
	return nil
}
