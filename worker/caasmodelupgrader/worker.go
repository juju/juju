// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelupgrader

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/status"
	jujuworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/gate"
)

var logger = loggo.GetLogger("juju.worker.modelupgrader")

// Facade exposes capabilities required by the worker.
type Facade interface {
	SetModelStatus(names.ModelTag, status.Status, string, map[string]interface{}) error
}

// Config holds the configuration and dependencies for a worker.
type Config struct {
	// Facade holds the API facade used by this worker for getting,
	// setting and watching the model's environ version.
	Facade Facade

	// GateUnlocker holds a gate.Unlocker that the worker must call
	// after the model has been successfully upgraded.
	GateUnlocker gate.Unlocker

	// ModelTag holds the tag of the model to which this worker is
	// scoped.
	ModelTag names.ModelTag
}

// Validate returns an error if the config cannot be expected
// to drive a functional worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.GateUnlocker == nil {
		return errors.NotValidf("nil GateUnlocker")
	}
	if config.ModelTag == (names.ModelTag{}) {
		return errors.NotValidf("empty ModelTag")
	}
	return nil
}

// NewWorker returns a worker that unlocks the model upgrade gate.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	// There are no upgrade steps for a CAAS model.
	// We just set the status to available and unlock the gate.
	return jujuworker.NewSimpleWorker(func(<-chan struct{}) error {
		setStatus := func(s status.Status, info string) error {
			return config.Facade.SetModelStatus(config.ModelTag, s, info, nil)
		}
		if err := setStatus(status.Available, ""); err != nil {
			return errors.Trace(err)
		}
		config.GateUnlocker.Unlock()
		return nil
	}), nil
}
