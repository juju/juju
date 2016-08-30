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

- model is dying/dead
- pending reboots
- model is being imported as part of another migration

## Source controller

- controller model is dying/dead
- pending reboots

## Target controller

- controller model is dying/dead
- pending reboots
- target controller already has a model with the same owner:name
- target controller already has a model with the same UUID
  - what about if left over from previous failed attempt? check model migration status

*/

// PrecheckBackend defines the interface to query Juju's state
// for migration prechecks.
type PrecheckBackend interface {
	AgentVersion() (version.Number, error)
	NeedsCleanup() (bool, error)
	IsUpgrading() (bool, error)
	AllMachines() ([]PrecheckMachine, error)
	AllApplications() ([]PrecheckApplication, error)
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

// PrecheckApplication describes state interface for an application
// needed by migration prechecks.
type PrecheckApplication interface {
	Name() string
	Life() state.Life
	AllUnits() ([]PrecheckUnit, error)
	MinUnits() int
}

// PrecheckUnit describes state interface for a unit needed by
// migration prechecks.
type PrecheckUnit interface {
	Name() string
	AgentTools() (*tools.Tools, error)
	Life() state.Life
	AgentStatus() (status.StatusInfo, error)
}

// SourcePrecheck checks the state of the source controller to make
// sure that the preconditions for model migration are met. The
// backend provided must be for the model to be migrated.
func SourcePrecheck(backend PrecheckBackend) error {
	// Check the model.
	if err := checkMachines(backend); err != nil {
		return errors.Trace(err)
	}

	if err := checkApplications(backend); err != nil {
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

func checkApplications(backend PrecheckBackend) error {
	modelVersion, err := backend.AgentVersion()
	if err != nil {
		return errors.Annotate(err, "retrieving model version")
	}
	apps, err := backend.AllApplications()
	if err != nil {
		return errors.Annotate(err, "retrieving applications")
	}
	for _, app := range apps {
		if app.Life() != state.Alive {
			return errors.Errorf("application %s is %s", app.Name(), app.Life())
		}
		err := checkUnits(app, modelVersion)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func checkUnits(app PrecheckApplication, modelVersion version.Number) error {
	units, err := app.AllUnits()
	if err != nil {
		return errors.Annotatef(err, "retrieving units for %s", app.Name())
	}
	if len(units) < app.MinUnits() {
		return errors.Errorf("application %s is below its minimum units threshold", app.Name())
	}

	for _, unit := range units {
		if unit.Life() != state.Alive {
			return errors.Errorf("unit %s is %s", unit.Name(), unit.Life())
		}

		if statusInfo, err := unit.AgentStatus(); err != nil {
			return errors.Annotatef(err, "retrieving unit %s status", unit.Name())
		} else if statusInfo.Status != status.StatusIdle {
			return newStatusError("unit %s not idle", unit.Name(), statusInfo.Status)
		}

		tools, err := unit.AgentTools()
		if err != nil {
			return errors.Annotatef(err, "retrieving tools for unit %s", unit.Name())
		}
		unitVersion := tools.Version.Number
		if unitVersion != modelVersion {
			return errors.Errorf("unit %s tools don't match model (%s != %s)",
				unit.Name(), unitVersion, modelVersion)
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
