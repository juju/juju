// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/state"
)

// MachineService provides access to all machines.
type MachineService interface {
	// AllMachines returns all machines in the model.
	AllMachines() ([]Machine, error)
}

// Machine defines machine methods needed for the check.
type Machine interface {
	// IsManual returns true if the machine was manually provisioned.
	IsManual() (bool, error)

	// IsContainer returns true if the machine is a container.
	IsContainer() bool

	// InstanceId returns the provider specific instance id for this
	// machine, or a NotProvisionedError, if not set.
	InstanceId() (instance.Id, error)

	// Id returns the machine id.
	Id() string
}

// CloudProvider defines methods needed from the cloud provider to perform the check.
type CloudProvider interface {
	// AllInstances returns all instances currently known to the cloud provider.
	AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error)
}

type stateShim struct {
	*state.State
}

// NewMachineService creates a machine service to use, based on state.State.
func NewMachineService(p *state.State) MachineService {
	return stateShim{p}
}

// AllMachines implements MachineService.AllMachines.
func (st stateShim) AllMachines() ([]Machine, error) {
	machines, err := st.State.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]Machine, len(machines))
	for i, m := range machines {
		result[i] = m
	}
	return result, nil
}
