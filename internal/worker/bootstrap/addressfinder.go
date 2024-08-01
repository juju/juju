// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
)

// BootstrapAddressesConfig encapsulates the configuration options for the
// bootstrap addresses finder.
type BootstrapAddressesConfig struct {
	BootstrapInstanceID    instance.Id
	SystemState            SystemState
	CloudService           CloudService
	CredentialService      CredentialService
	ModelService           ModelService
	ModelConfigService     ModelConfigService
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
	controllerModel, err := config.ModelService.ControllerModel(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get controller model: %w", err)
	}

	env, err := getEnviron(
		ctx,
		controllerModel,
		config.ModelConfigService,
		config.CloudService,
		config.CredentialService,
		config.NewEnvironFunc)
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
// worker config. The new environ is based off of the model that is supplied.
func getEnviron(
	ctx context.Context,
	model coremodel.Model,
	modelConfigService ModelConfigService,
	cloudService CloudService,
	credentialService CredentialService,
	newEnviron NewEnvironFunc,
) (environs.Environ, error) {
	cred, err := getEnvironCredential(ctx, model, credentialService)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloud, err := cloudService.Cloud(ctx, model.Cloud)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cloudSpec, err := cloudspec.MakeCloudSpec(*cloud, model.CloudRegion, cred)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelConfig, err := modelConfigService.ModelConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot get model config to establish environ for model %q: %w",
			model.UUID,
			err,
		)
	}

	return newEnviron(ctx, environs.OpenParams{
		ControllerUUID: model.UUID.String(),
		Cloud:          cloudSpec,
		Config:         modelConfig,
	})
}

// getEnvironCredential returns the an environ credential for the given model.
func getEnvironCredential(
	ctx context.Context,
	model coremodel.Model,
	credentialService CredentialService,
) (*cloud.Credential, error) {
	credentialValue, err := credentialService.CloudCredential(ctx, model.Credential)
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
