// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5/catacomb"

	corelogger "github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/worker/logsender"
)

// LocalConfig contains the settings required by the local logsink backend.
type LocalConfig struct {
	BackendBufferSize int
	LogSink           corelogger.LogSink
}

// Validate checks that the local backend config is usable.
func (c LocalConfig) Validate() error {
	if c.BackendBufferSize <= 0 {
		return errors.NotValidf("non-positive BackendBufferSize")
	}
	if c.LogSink == nil {
		return errors.NotValidf("nil LogSink")
	}
	return nil
}

type localBackend struct {
	catacomb catacomb.Catacomb
	cfg      LocalConfig
	records  logsender.LogRecordCh
}

// NewLocal returns a backend that writes records to a local logsink.
func NewLocal(cfg LocalConfig) (Backend, error) {
	if err := cfg.Validate(); err != nil {
		return nil, internalerrors.Capture(err)
	}

	w := &localBackend{
		cfg:     cfg,
		records: make(logsender.LogRecordCh, cfg.BackendBufferSize),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "log-router-local",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, internalerrors.Capture(err)
	}
	return w, nil
}

// Kill stops the backend and closes the log record channel.
func (w *localBackend) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the backend to stop.
func (w *localBackend) Wait() error {
	return w.catacomb.Wait()
}

// LogRecords returns the channel on which log records are sent to the backend.
func (w *localBackend) LogRecords() logsender.LogRecordCh {
	return w.records
}

// Report returns a report of the local backend's current state.
func (w *localBackend) Report(_ context.Context) map[string]any {
	return map[string]any{
		"name":            "local-backend",
		"bufferedRecords": len(w.records),
	}
}

func (w *localBackend) loop() error {
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

			err := w.cfg.LogSink.Log([]corelogger.LogRecord{{
				Time:      rec.Time,
				Module:    rec.Module,
				Entity:    rec.Entity,
				Location:  rec.Location,
				Level:     corelogger.Level(rec.Level),
				Message:   rec.Message,
				Labels:    rec.Labels,
				ModelUUID: rec.ModelUUID,
			}})
			if err != nil {
				return internalerrors.Capture(err)
			}
		}
	}
}
