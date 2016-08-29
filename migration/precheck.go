// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools"
)

/*
# TODO - remaining prechecks

## Source model

- unit is being provisioned
- unit is dying/dead
- model is dying/dead
- application is being provisioned?
  * check unit count? possibly can't have unit count > 0 without a unit existing - check this
  * unit count of 0 is ok if service was deployed and then all units for it were removed.
  * from Will: I have a suspicion that GUI-deployed apps (and maybe
    others?) will have minunits of 1, meaning that a unit count of 0
    is not necessarily stable.
- pending reboots
- model is being imported as part of another migration

## Source controller

- controller model is dying/dead
- pending reboots

## Target controller

- controller model is dying/dead
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
	IsUpgrading() (bool, error)
	ControllerBackend() (PrecheckBackend, error)
}

// PrecheckMachine describes state interface for a machine needed by
// migration prechecks.
type PrecheckMachine interface {
	Id() string
	AgentTools() (*tools.Tools, error)
	Life() state.Life
	Status() (status.StatusInfo, error)
	InstanceStatus() (status.StatusInfo, error)
}

// SourcePrecheck checks the state of the source controller to make
// sure that the preconditions for model migration are met. The
// backend provided must be for the model to be migrated.
func SourcePrecheck(backend PrecheckBackend) error {
	// Check the model.
	if err := checkMachines(backend); err != nil {
		return errors.Trace(err)
	}

	if cleanupNeeded, err := backend.NeedsCleanup(); err != nil {
		return errors.Annotate(err, "checking cleanups")
	} else if cleanupNeeded {
		return errors.New("cleanup needed")
	}

	// Check the source controller.
	controllerBackend, err := backend.ControllerBackend()
	if err != nil {
		return errors.Trace(err)
	}
	if err := checkController(controllerBackend); err != nil {
		return errors.Annotate(err, "controller")
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

	err = checkController(backend)
	return errors.Trace(err)
}

func checkController(backend PrecheckBackend) error {
	if upgrading, err := backend.IsUpgrading(); err != nil {
		return errors.Annotate(err, "checking for upgrades")
	} else if upgrading {
		return errors.New("upgrade in progress")
	}

	err := checkMachines(backend)
	return errors.Trace(err)
}

func checkMachines(backend PrecheckBackend) error {
	modelVersion, err := backend.AgentVersion()
	if err != nil {
		return errors.Annotate(err, "retrieving model version")
	}

	machines, err := backend.AllMachines()
	if err != nil {
		return errors.Annotate(err, "retrieving machines")
	}
	for _, machine := range machines {
		if machine.Life() != state.Alive {
			return errors.Errorf("machine %s is %s", machine.Id(), machine.Life())
		}

		if statusInfo, err := machine.InstanceStatus(); err != nil {
			return errors.Annotatef(err, "retrieving machine %s instance status", machine.Id())
		} else if statusInfo.Status != status.StatusRunning {
			return newStatusError("machine %s not running", machine.Id(), statusInfo.Status)
		}

		if statusInfo, err := machine.Status(); err != nil {
			return errors.Annotatef(err, "retrieving machine %s status", machine.Id())
		} else if statusInfo.Status != status.StatusStarted {
			return newStatusError("machine %s not started", machine.Id(), statusInfo.Status)
		}

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

func newStatusError(format, id string, s status.Status) error {
	msg := fmt.Sprintf(format, id)
	if s != status.StatusEmpty {
		msg += fmt.Sprintf(" (%s)", s)
	}
	return errors.New(msg)
}
