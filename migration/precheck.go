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

- controller is upgrading
  * all machine versions must match agent version
- source controller has upgrade info doc (IsUpgrading)
- controller machines have errors
- controller machines that are dying or dead
- pending reboots

## Target controller

- target controller tools are less than source model tools
- target controller machines have errors
- target controller already has a model with the same owner:name
- target controller already has a model with the same UUID
  - what about if left over from previous failed attempt? check model migration status
- source controller has upgrade info doc (IsUpgrading)

*/

// PrecheckBackend defines the interface to query Juju's state
// for migration prechecks.
type PrecheckBackend interface {
	NeedsCleanup() (bool, error)
	AgentVersion() (version.Number, error)
	AllMachines() ([]PrecheckMachine, error)
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
	cleanupNeeded, err := backend.NeedsCleanup()
	if err != nil {
		return errors.Annotate(err, "checking cleanups")
	}
	if cleanupNeeded {
		return errors.New("cleanup needed")
	}

	modelVersion, err := backend.AgentVersion()
	if err != nil {
		return errors.Annotate(err, "retrieving model version")
	}

	err = checkMachines(backend, modelVersion)
	return errors.Trace(err)
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
