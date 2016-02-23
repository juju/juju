// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

// WorkerFactory exposes functionality for creating workers
// for an agent.
type WorkerFactory interface {
	// NewModelWorker returns a "new worker" func that may be used to
	// start a state worker for the state's model. If model workers are
	// not supported then false is returned (for "supported").
	NewModelWorker(name string, st *state.State) (newWorker func() (worker.Worker, error), supported bool)
}

var registeredWorkers = map[string]WorkerFactory{}

// RegisterWorker adds a worker factory for the named worker
// to the registry. If the name is already registered then
// errors.AlreadyExists is returned.
func RegisterWorker(name string, factory WorkerFactory) error {
	if existing, ok := registeredWorkers[name]; ok && factory != existing {
		return errors.NewAlreadyExists(nil, fmt.Sprintf("worker %q already registered", name))
	}
	registeredWorkers[name] = factory
	return nil
}
