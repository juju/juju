// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/state"
)

// PersistentBackend defines persisted entities that are accessed
// during credential validity check.
type PersistentBackend interface {
	// Model returns the model entity.
	Model() (Model, error)

	// Cloud returns the controller's cloud definition.
	Cloud(name string) (cloud.Cloud, error)

	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(tag names.CloudCredentialTag) (state.Credential, error)

	// AllMachines returns all machines in the model.
	AllMachines() ([]Machine, error)

	// ControllerConfig returns controller config.
	ControllerConfig() (ControllerConfig, error)
}

// Model defines model methods needed for the check.
type Model interface {
	// CloudName returns the name of the cloud to which the model is deployed.
	CloudName() string

	// CloudRegion returns the name of the cloud region to which the model is deployed.
	CloudRegion() string

	// Config returns the config for the model.
	Config() (*config.Config, error)

	// ValidateCloudCredential validates new cloud credential for this model.
	ValidateCloudCredential(tag names.CloudCredentialTag, credential cloud.Credential) error

	// Type returns the type of the model.
	Type() state.ModelType

	// CloudCredentialTag returns the tag of the cloud credential used for managing the
	// model's cloud resources, and a boolean indicating whether a credential is set.
	CloudCredentialTag() (names.CloudCredentialTag, bool)
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

// ControllerConfig defines methods needed from the cloud provider to perform the check.
type ControllerConfig interface {
	ControllerUUID() string
}

type stateShim struct {
	*state.State
}

// NewPersistentBackend creates a credential validity backend to use, based on state.State.
func NewPersistentBackend(p *state.State) PersistentBackend {
	return stateShim{p}
}

// AllMachines implements PersistentBackend.AllMachines.
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

// Model implements PersistentBackend.Model.
func (st stateShim) Model() (Model, error) {
	m, err := st.State.Model()
	return m, err
}

// Model implements PersistentBackend.Model.
func (st stateShim) ControllerConfig() (ControllerConfig, error) {
	return st.State.ControllerConfig()
}
