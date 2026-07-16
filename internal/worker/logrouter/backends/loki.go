// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	"github.com/prometheus/client_golang/prometheus"

	corelogger "github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/loki"
	"github.com/juju/juju/internal/worker/logsender"
)

// LokiConfig contains the settings required by the Loki backend.
type LokiConfig struct {
	BackendBufferSize    int
	ClientConfig         loki.Config
	Endpoint             string
	ControllerUUID       string
	ModelUUID            string
	AgentID              string
	ServiceName          string
	PrometheusRegisterer prometheus.Registerer
	NewClient            NewLokiClientFunc
}

// Validate checks that the Loki backend config is usable.
func (c LokiConfig) Validate() error {
	if c.BackendBufferSize <= 0 {
		return errors.NotValidf("non-positive BackendBufferSize")
	}
	if c.Endpoint == "" {
		return errors.NotValidf("empty Endpoint")
	}
	if c.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if c.ServiceName == "" {
		return errors.NotValidf("empty ServiceName")
	}
	if c.NewClient == nil {
		return errors.NotValidf("nil NewClient")
	}
	return nil
}

// LokiClient is the Loki push client surface used by the backend.
type LokiClient interface {
	worker.Worker
	Push(...loki.Record) error
	Report(ctx context.Context) map[string]any
}

// NewLokiClientFunc returns a Loki client worker.
type NewLokiClientFunc func(string, loki.Config) (LokiClient, error)

type lokiBackend struct {
	catacomb         catacomb.Catacomb
	cfg              LokiConfig
	client           LokiClient
	metricsCollector *loki.Collector
	records          logsender.LogRecordCh
}

// NewLoki returns a backend that sends log records to a Loki endpoint.
func NewLoki(cfg LokiConfig) (Backend, error) {
	if err := cfg.Validate(); err != nil {
		return nil, internalerrors.Capture(err)
	}

	client, err := cfg.NewClient(cfg.Endpoint, cfg.ClientConfig)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	source, ok := client.(loki.MetricsSource)
	if !ok {
		client.Kill()
		_ = client.Wait()
		return nil, errors.NotValidf("loki client metrics source")
	}
	metricsCollector := loki.NewMetricsCollector(source)
	_ = cfg.PrometheusRegisterer.Unregister(metricsCollector)
	if err := cfg.PrometheusRegisterer.Register(metricsCollector); err != nil {
		client.Kill()
		_ = client.Wait()
		return nil, internalerrors.Capture(err)
	}
	w := &lokiBackend{
		cfg:              cfg,
		client:           client,
		metricsCollector: metricsCollector,
		records:          make(logsender.LogRecordCh, cfg.BackendBufferSize),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "log-router-loki",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			client,
		},
	}); err != nil {
		_ = cfg.PrometheusRegisterer.Unregister(metricsCollector)
		client.Kill()
		_ = client.Wait()
		return nil, internalerrors.Capture(err)
	}
	return w, nil
}

// Kill stops the backend and closes the log record channel.
func (w *lokiBackend) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the backend to stop.
func (w *lokiBackend) Wait() error {
	return w.catacomb.Wait()
}

// LogRecords returns the channel that the log router will send log records to.
func (w *lokiBackend) LogRecords() logsender.LogRecordCh {
	return w.records
}

// Log implements corelogger.LogSink by converting records to the internal
// logsender format and submitting them to the backend's record channel.
func (w *lokiBackend) Log(records []corelogger.LogRecord) error {
	return sendRecords(w.records, records)
}

// WatchRefresh implements corelogger.LogSink. Individual backends never
// change their underlying target; refresh signalling is handled by the log
// router when switching backends.
func (w *lokiBackend) WatchRefresh() <-chan struct{} {
	return corelogger.NoRefresh()
}

// Report returns a report of the backend's current state.
func (w *lokiBackend) Report(ctx context.Context) map[string]any {
	return map[string]any{
		"name":         "loki-backend",
		"service_name": w.cfg.ServiceName,
		"client":       w.client.Report(ctx),
	}
}

func (w *lokiBackend) loop() error {
	defer func() {
		_ = w.cfg.PrometheusRegisterer.Unregister(w.metricsCollector)
	}()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case rec, ok := <-w.records:
			if !ok {
				return nil
			}
			if rec == nil {
				continue
			}
			modelUUID := w.cfg.ModelUUID
			if rec.ModelUUID != "" {
				modelUUID = rec.ModelUUID
			}
			agentID := w.cfg.AgentID
			if rec.Entity != "" {
				agentID = rec.Entity
			}
			if err := w.client.Push(loki.Record{
				Timestamp:      rec.Time,
				Line:           rec.Message,
				ControllerUUID: w.cfg.ControllerUUID,
				ModelUUID:      modelUUID,
				AgentID:        agentID,
				Fields: map[string]string{
					"module":   rec.Module,
					"location": rec.Location,
					"level":    rec.Level.String(),
				},
				ServiceName: w.cfg.ServiceName,
				TraceID:     rec.Labels["trace_id"],
				SpanID:      rec.Labels["span_id"],
			}); err != nil {
				return internalerrors.Capture(err)
			}
		}
	}
}
