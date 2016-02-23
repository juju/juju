// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

type modelWorkerFactoryFunc func(st *state.State) func() (worker.Worker, error)

var registeredModelWorkers = map[string]modelWorkerFactoryFunc{}

// RegisterModelWorker adds a worker factory for the named worker
// to the registry. If the name is already registered then
// errors.AlreadyExists is returned.
func RegisterModelWorker(name string, factory modelWorkerFactoryFunc) error {
	if _, ok := registeredModelWorkers[name]; ok {
		return errors.NewAlreadyExists(nil, fmt.Sprintf("worker %q already registered", name))
	}
	registeredModelWorkers[name] = factory
	return nil
}
