// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/bootstrap"
)

// BootstrapAddressesConfig encapsulates the configuration options for the
// bootstrap addresses finder.
type BootstrapAddressesConfig struct {
	BootstrapInstanceID    instance.Id
	SystemState            SystemState
	CloudService           CloudService
	CredentialService      CredentialService
	NewEnvironFunc         NewEnvironFunc
	BootstrapAddressesFunc BootstrapAddressesFunc
}

// CAASBootstrapAddressFinder returns the bootstrap addresses for CAAS. We know
// that currently CAAS doesn't implement InstanceListener, so we return
// localhost.
func CAASBootstrapAddressFinder(ctx context.Context, config BootstrapAddressesConfig) (network.ProviderAddresses, error) {
	return network.NewMachineAddresses([]string{"localhost"}).AsProviderAddresses(), nil
}

// CAASNewEnviron returns a new environ for CAAS. We know that currently CAAS
// doesn't implement InstanceListener, so we return an error.
func CAASNewEnviron(ctx context.Context, params environs.OpenParams) (environs.Environ, error) {
	return nil, errors.NotSupportedf("new environ")
}

// CAASBootstrapAddresses returns the bootstrap addresses for CAAS. We know that
// currently CAAS doesn't implement InstanceListener, so we return an error.
func CAASBootstrapAddresses(ctx context.Context, env environs.Environ, bootstrapInstanceID instance.Id) (network.ProviderAddresses, error) {
	return nil, errors.NotSupportedf("bootstrap addresses")
}

// IAASBootstrapAddressFinder returns the bootstrap addresses for IAAS.
func IAASBootstrapAddressFinder(ctx context.Context, config BootstrapAddressesConfig) (network.ProviderAddresses, error) {
	env, err := getEnviron(ctx, config.SystemState, config.CloudService, config.CredentialService, config.NewEnvironFunc)
	if err != nil {
		return nil, errors.Trace(err)
	}
	addresses, err := config.BootstrapAddressesFunc(ctx, env, config.BootstrapInstanceID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return addresses, nil
}

// getEnviron creates a new environ using the provided NewEnvironFunc from the
// worker config.
func getEnviron(ctx context.Context, state SystemState, cloudService CloudService, credentialService CredentialService, newEnviron NewEnvironFunc) (environs.Environ, error) {
	controllerModel, err := state.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	cred, err := getEnvironCredential(ctx, controllerModel, credentialService)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloud, err := cloudService.Cloud(ctx, controllerModel.CloudName())
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

	return newEnviron(ctx, environs.OpenParams{
		ControllerUUID: state.ControllerModelUUID(),
		Cloud:          cloudSpec,
		Config:         controllerModelConfig,
	})
}

func getEnvironCredential(ctx context.Context, controllerModel bootstrap.Model, credentialService CredentialService) (*cloud.Credential, error) {
	var cred *cloud.Credential
	if cloudCredentialTag, ok := controllerModel.CloudCredentialTag(); ok {
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
		cred = &cloudCredential
	}

	return cred, nil
}
