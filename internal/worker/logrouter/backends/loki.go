// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/internal/loki"
	"github.com/juju/juju/internal/worker/logsender"
)

// LokiConfig contains the settings required by the Loki backend.
type LokiConfig struct {
	BackendBufferSize int
	ClientConfig      loki.Config
	Endpoint          string
	ControllerUUID    string
	ModelUUID         string
	AgentID           string
	NewClient         NewLokiClientFunc
}

// LokiClient is the Loki push client surface used by the backend.
type LokiClient interface {
	worker.Worker
	Push(...loki.Record) error
}

// NewLokiClientFunc returns a Loki client worker.
type NewLokiClientFunc func(string, loki.Config) (LokiClient, error)

type lokiBackend struct {
	catacomb catacomb.Catacomb
	cfg      LokiConfig
	client   LokiClient
	records  logsender.LogRecordCh
}

// NewLoki returns a backend that sends log records to a Loki endpoint.
func NewLoki(cfg LokiConfig) (Backend, error) {
	client, err := cfg.NewClient(cfg.Endpoint, cfg.ClientConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w := &lokiBackend{
		cfg:     cfg,
		client:  client,
		records: make(logsender.LogRecordCh, cfg.BackendBufferSize),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "log-router-loki",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			client,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (w *lokiBackend) Kill() {
	w.catacomb.Kill(nil)
}

func (w *lokiBackend) Wait() error {
	return w.catacomb.Wait()
}

func (w *lokiBackend) Dying() <-chan struct{} {
	return w.catacomb.Dying()
}

func (w *lokiBackend) LogRecords() logsender.LogRecordCh {
	return w.records
}

func (w *lokiBackend) loop() error {
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
			if err := w.client.Push(loki.Record{
				Timestamp:      rec.Time,
				Line:           rec.Message,
				ControllerUUID: w.cfg.ControllerUUID,
				ModelUUID:      w.cfg.ModelUUID,
				AgentID:        w.cfg.AgentID,
				Fields: map[string]string{
					"module":   rec.Module,
					"location": rec.Location,
					"level":    rec.Level.String(),
				},
			}); err != nil {
				return errors.Trace(err)
			}
		}
	}
}
