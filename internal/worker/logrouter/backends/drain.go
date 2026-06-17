// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"fmt"

	"github.com/juju/worker/v5/catacomb"

	internalerrors "github.com/juju/juju/internal/errors"
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
		return nil, internalerrors.Capture(err)
	}
	return w, nil
}

// Kill stops the backend and closes the log record channel.
func (w *drainBackend) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the backend to stop.
func (w *drainBackend) Wait() error {
	return w.catacomb.Wait()
}

// LogRecords returns the channel on which log records are sent to the backend.
func (w *drainBackend) LogRecords() logsender.LogRecordCh {
	return w.records
}

func (w *drainBackend) loop() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case r, ok := <-w.records:
			if !ok {
				return nil
			}
			fmt.Println("drainBackend loop: received record", r.Message)
		}
	}
}
