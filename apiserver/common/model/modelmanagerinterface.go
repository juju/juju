// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	domainstatus "github.com/juju/juju/domain/status"
)

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// AllMachineNames returns the names of all machines in the model.
	AllMachineNames(context.Context) ([]machine.Name, error)

	// GetMachineLife returns the GetMachineLife status of the specified machine.
	GetMachineLife(context.Context, machine.Name) (life.Value, error)

	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(context.Context, machine.Name) (machine.UUID, error)

	// GetInstanceIDAndName returns the cloud specific instance ID and display name for
	// this machine.
	GetInstanceIDAndName(context.Context, machine.UUID) (instance.Id, string, error)

	// GetHardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	GetHardwareCharacteristics(context.Context, machine.UUID) (*instance.HardwareCharacteristics, error)

	// GetSupportedContainersTypes returns the supported container types for the
	// provider.
	GetSupportedContainersTypes(context.Context, machine.UUID) ([]instance.ContainerType, error)

	// WatchModelMachines watches for additions or updates to non-container
	// machines. It is used by workers that need to factor life value changes,
	// and so does not factor machine removals, which are considered to be
	// after their transition to the dead state.
	// It emits machine names rather than UUIDs.
	WatchModelMachines(context.Context) (watcher.StringsWatcher, error)

	// WatchModelMachineLifeAndStartTimes returns a string watcher that emits machine names
	// for changes to machine life or agent start times.
	WatchModelMachineLifeAndStartTimes(context.Context) (watcher.StringsWatcher, error)
}

// StatusService returns the status of a applications, and units and machines.
type StatusService interface {
	// GetApplicationAndUnitModelStatuses returns the application name and unit
	// count for each model for the model status request.
	GetApplicationAndUnitModelStatuses(context.Context) (map[string]int, error)

	// GetModelStatusInfo returns only basic model information used for
	// displaying model status.
	// The following error types can be expected to be returned:
	// - [modelerrors.NotFound]: When the model does not exist.
	GetModelStatusInfo(context.Context) (domainstatus.ModelStatusInfo, error)

	// GetAllMachineStatuses returns all the machine statuses for the model, indexed
	// by machine name.
	GetAllMachineStatuses(context.Context) (map[machine.Name]status.StatusInfo, error)
}
