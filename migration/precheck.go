// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/tools"
)

/*
# TODO - remaining prechecks

## Source model

- model machines have errors
- machines that are dying or dead
- pending reboots
- machine or unit is being provisioned
- application is being provisioned?
- units that are dying or dead
- model is being imported as part of another migration

## Source controller

- source controller has upgrade info doc (IsUpgrading)
- controller machines have errors
- controller machines that are dying or dead
- machine or unit is being provisioned
- application is being provisioned?
- pending reboots

## Target controller

- target controller has upgrade info doc (IsUpgrading)
- target controller machines have errors
- target controller already has a model with the same owner:name
- target controller already has a model with the same UUID
  - what about if left over from previous failed attempt? check model migration status

*/

// PrecheckBackend defines the interface to query Juju's state
// for migration prechecks.
type PrecheckBackend interface {
	NeedsCleanup() (bool, error)
	AgentVersion() (version.Number, error)
	AllMachines() ([]PrecheckMachine, error)
	ControllerBackend() (PrecheckBackend, error)
}

// PrecheckMachine describes state interface for a machine needed by
// migration prechecks.
type PrecheckMachine interface {
	Id() string
	AgentTools() (*tools.Tools, error)
}

// SourcePrecheck checks the state of the source controller to make
// sure that the preconditions for model migration are met. The
// backend provided must be for the model to be migrated.
func SourcePrecheck(backend PrecheckBackend) error {
	if cleanupNeeded, err := backend.NeedsCleanup(); err != nil {
		return errors.Annotate(err, "checking cleanups")
	} else if cleanupNeeded {
		return errors.New("cleanup needed")
	}

	if err := checkSourceMachines(backend); err != nil {
		return errors.Trace(err)
	}

	// Now check the source controller.
	controllerBackend, err := backend.ControllerBackend()
	if err != nil {
		return errors.Trace(err)
	}
	if err := checkSourceMachines(controllerBackend); err != nil {
		return errors.Annotate(err, "source controller")
	}
	return nil
}

func checkSourceMachines(backend PrecheckBackend) error {
	modelVersion, err := backend.AgentVersion()
	if err != nil {
		return errors.Annotate(err, "retrieving model version")
	}
	if err := checkMachines(backend, modelVersion); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// TargetPrecheck checks the state of the target controller to make
// sure that the preconditions for model migration are met. The
// backend provided must be for the target controller.
func TargetPrecheck(backend PrecheckBackend, modelVersion version.Number) error {
	controllerVersion, err := backend.AgentVersion()
	if err != nil {
		return errors.Annotate(err, "retrieving model version")
	}

	if controllerVersion.Compare(modelVersion) < 0 {
		return errors.Errorf("model has higher version than target controller (%s > %s)",
			modelVersion, controllerVersion)
	}

	err = checkMachines(backend, controllerVersion)
	return errors.Trace(err)
}

func checkMachines(backend PrecheckBackend, modelVersion version.Number) error {
	machines, err := backend.AllMachines()
	if err != nil {
		return errors.Annotate(err, "retrieving machines")
	}
	for _, machine := range machines {
		tools, err := machine.AgentTools()
		if err != nil {
			return errors.Annotatef(err, "retrieving tools for machine %s", machine.Id())
		}
		machineVersion := tools.Version.Number
		if machineVersion != modelVersion {
			return errors.Errorf("machine %s tools don't match model (%s != %s)",
				machine.Id(), machineVersion, modelVersion)
		}
	}
	return nil
}
