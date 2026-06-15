// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/internal/worker/logsender"
)

type drainBackend struct {
	catacomb catacomb.Catacomb
	records  logsender.LogRecordCh
}

// NewDrain returns a backend that drains log records without sending them
// anywhere.
func NewDrain(backendBufferSize int) (Backend, error) {
	w := &drainBackend{
		records: make(logsender.LogRecordCh, backendBufferSize),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "log-router-drain",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (w *drainBackend) Kill() {
	w.catacomb.Kill(nil)
}

func (w *drainBackend) Wait() error {
	return w.catacomb.Wait()
}

func (w *drainBackend) Dying() <-chan struct{} {
	return w.catacomb.Dying()
}

func (w *drainBackend) LogRecords() logsender.LogRecordCh {
	return w.records
}

func (w *drainBackend) loop() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-w.records:
			if !ok {
				return nil
			}
		}
	}
}
