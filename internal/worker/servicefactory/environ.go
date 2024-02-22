// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/domain/credential"
	domainservicefactory "github.com/juju/juju/domain/servicefactory"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// SystemState is the interface that is used to get the system state.
// Deprecated: Move these to the domain service.
type SystemState interface {
	// ControllerModelUUID returns the UUID of the model that was
	// bootstrapped.  This is the only model that can have controller
	// machines.  The owner of this model is also considered "special", in
	// that they are the only user that is able to create other users
	// (until we have more fine grained permissions), and they cannot be
	// disabled.
	ControllerModelUUID() string
	// Model returns the model.
	Model() (Model, error)
	// Release ensures that we release the system state back to the tracker.
	Release() error
}

// CredentialService is the interface that is used to get the
// cloud credential.
type CredentialService interface {
	CloudCredential(ctx context.Context, id credential.ID) (cloud.Credential, error)
}

// CloudService is the interface that is used to get the cloud service
// for the controller.
type CloudService interface {
	Get(context.Context, string) (*cloud.Cloud, error)
}

// Model is the interface that is used to get information about a model.
type Model interface {
	Config() (*config.Config, error)
	CloudCredentialTag() (names.CloudCredentialTag, bool)
	CloudRegion() string
	CloudName() string
}

// ControllerServiceFactory is the interface that is used to get the
// controller service factory.
type ControllerServiceFactory interface {
	Credential() CredentialService
	Cloud() CloudService
}

// NewEnvironFunc is the function that is used to create a new environ.
type NewEnvironFunc func(context.Context, environs.OpenParams) (environs.Environ, error)

// CAASNewEnviron returns a new environ for CAAS. We know that currently CAAS
// doesn't implement InstanceListener, so we return an error.
func CAASNewEnviron(ctx context.Context, params environs.OpenParams) (environs.Environ, error) {
	return nil, errors.NotSupportedf("new environ")
}

// EnvironConfig is used to configure an environ.
type EnvironConfig struct {
	newEnviron  NewEnvironFunc
	systemState SystemState
}

// NewEnvironConfig returns a new environ builder.
func NewEnvironConfig(newEnviron NewEnvironFunc, systemState SystemState) EnvironConfig {
	return EnvironConfig{
		newEnviron:  newEnviron,
		systemState: systemState,
	}
}

// WithControllerServiceFactory returns a new environ factory using the existing
// config.
func (f EnvironConfig) WithControllerServiceFactory(service ControllerServiceFactory) domainservicefactory.EnvironFactory {
	return EnvironFactory{
		newEnviron:     f.newEnviron,
		systemState:    f.systemState,
		serviceFactory: service,
	}
}

// EnvironFactory provides access to the environment identified by the
// environment UUID.
type EnvironFactory struct {
	newEnviron     NewEnvironFunc
	systemState    SystemState
	serviceFactory ControllerServiceFactory
}

// Environ returns a new environ.
func (f EnvironFactory) Environ(ctx context.Context) (environs.BootstrapEnviron, error) {
	controllerModel, err := f.systemState.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	cred, err := getEnvironCredential(ctx, controllerModel, f.serviceFactory.Credential())
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloud, err := f.serviceFactory.Cloud().Get(ctx, controllerModel.CloudName())
	if err != nil {
		return nil, errors.Trace(err)
	}

	cloudSpec, err := cloudspec.MakeCloudSpec(*cloud, controllerModel.CloudRegion(), cred)
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerModelConfig, err := controllerModel.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return f.newEnviron(ctx, environs.OpenParams{
		ControllerUUID: f.systemState.ControllerModelUUID(),
		Cloud:          cloudSpec,
		Config:         controllerModelConfig,
	})
}

func getEnvironCredential(ctx context.Context, controllerModel Model, credentialService CredentialService) (*cloud.Credential, error) {
	cloudCredentialTag, ok := controllerModel.CloudCredentialTag()
	if !ok {
		return nil, nil
	}

	credentialValue, err := credentialService.CloudCredential(ctx, credential.IdFromTag(cloudCredentialTag))
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudCredential := cloud.NewNamedCredential(
		credentialValue.Label,
		credentialValue.AuthType(),
		credentialValue.Attributes(),
		credentialValue.Revoked,
	)
	return &cloudCredential, nil
}

// stateShim converts a state.State to a SystemState.
type stateShim struct {
	*state.State
	releaser func() error
}

func (s stateShim) Model() (Model, error) {
	m, err := s.State.Model()
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s stateShim) Release() error {
	return s.releaser()
}
