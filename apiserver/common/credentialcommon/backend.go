// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

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

// CloudEntitiesBackend defines what cloud entities where persisted in state
// and will be accessed during the check.
type CloudEntitiesBackend interface {
	// AllMachines returns all machines in the model.
	AllMachines() ([]Machine, error)
}

// Model defines model methods needed for the check.
type Model interface {
	// Cloud returns the name of the cloud to which the model is deployed.
	Cloud() string

	// CloudRegion returns the name of the cloud region to which the model is deployed.
	CloudRegion() string

	// Config returns the config for the model.
	Config() (*config.Config, error)

	// ValidateCloudCredential validates new cloud credential for this model.
	ValidateCloudCredential(tag names.CloudCredentialTag, credential cloud.Credential) error
}

// ModelBackend defines what model specific properties where persisted in state
// and will be accessed during the check.
type ModelBackend interface {
	CloudEntitiesBackend

	// Model returns the model entity.
	Model() (Model, error)

	// Cloud returns the controller's cloud definition.
	Cloud(name string) (cloud.Cloud, error)
}

type stateShim struct {
	*state.State
}

// NewCloudEntitiesBackend creates a backend to use based on state.State.
func NewCloudEntitiesBackend(p *state.State) CloudEntitiesBackend {
	return stateShim{p}
}

// NewModelBackend creates a model backend to use based on state.State.
func NewModelBackend(p *state.State) ModelBackend {
	return stateShim{p}
}

// AllMachines implements PersistedBackend.AllMachines.
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

// Model implements PersistedBackend.Model.
func (st stateShim) Model() (Model, error) {
	m, err := st.State.Model()
	return m, err
}

// CloudProvider defines methods needed from the cloud provider to perform the check.
type CloudProvider interface {
	// AllInstances returns all instances currently known to the cloud provider.
	AllInstances(ctx context.ProviderCallContext) ([]instance.Instance, error)
}
