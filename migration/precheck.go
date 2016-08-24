// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/tools"
)

// SourcePrecheckBackend defines the interface to query Juju's state
// for migration prechecks.
type SourcePrecheckBackend interface {
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
func SourcePrecheck(backend SourcePrecheckBackend) error {
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
