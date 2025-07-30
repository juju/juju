// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
}

type ModelInfoService interface {
	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(ctx context.Context) (model.ModelInfo, error)
}

// EnvironConfigGetter implements environs.EnvironConfigGetter
// in terms of a Model.
type EnvironConfigGetter struct {
	// NewContainerBroker is a func that returns a caas container broker
	// for the relevant model.
	NewContainerBroker caas.NewContainerBrokerFunc

	ModelConfigService ModelConfigService

	// CredentialService provides access to credentials.
	CredentialService CredentialService

	// CloudService provides access to clouds.
	CloudService CloudService

	ModelInfoService ModelInfoService
}

// ControllerUUID returns the universally unique identifier of the controller.
func (g EnvironConfigGetter) ControllerUUID(ctx context.Context) (string, error) {
	modelInfo, err := g.ModelInfoService.GetModelInfo(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	return modelInfo.ControllerUUID.String(), nil
}

// ModelConfig implements environs.EnvironConfigGetter.
func (g EnvironConfigGetter) ModelConfig(ctx context.Context) (*config.Config, error) {
	return g.ModelConfigService.ModelConfig(ctx)
}

// CloudSpec implements environs.EnvironConfigGetter.
func (g EnvironConfigGetter) CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error) {
	return CloudSpecForModel(ctx, g.ModelInfoService, g.CloudService, g.CredentialService)
}

// CloudSpecForModel returns a CloudSpec for the specified model.
func CloudSpecForModel(
	ctx context.Context,
	modelInfoService ModelInfoService,
	cloudService CloudService,
	credentialService CredentialService,
) (environscloudspec.CloudSpec, error) {
	modelInfo, err := modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}

	cld, err := cloudService.Cloud(ctx, modelInfo.Cloud)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	regionName := modelInfo.CloudRegion
	credentialKey := credential.Key{
		Cloud: modelInfo.Cloud,
		Owner: model.ControllerModelOwnerUsername,
		Name:  modelInfo.CredentialName,
	}
	cred, err := credentialService.CloudCredential(ctx, credentialKey)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	return environscloudspec.MakeCloudSpec(*cld, regionName, &cred)
}

// NewCAASBrokerFunc aliases a function that, given a state.State,
// returns a new CAAS broker.
type NewCAASBrokerFunc = func(ModelInfoService, CloudService, CredentialService, ModelConfigService) (caas.Broker, error)

// GetNewCAASBrokerFunc returns a NewCAASBrokerFunc, that constructs CAAS brokers
// using the given caas.NewContainerBrokerFunc.
func GetNewCAASBrokerFunc(newBroker caas.NewContainerBrokerFunc) NewCAASBrokerFunc {
	return func(modelInfoService ModelInfoService, cloudService CloudService, credentialService CredentialService, modelConfigService ModelConfigService) (caas.Broker, error) {
		g := EnvironConfigGetter{ModelInfoService: modelInfoService, CloudService: cloudService, CredentialService: credentialService, ModelConfigService: modelConfigService}
		cloudSpec, err := g.CloudSpec(context.TODO())
		if err != nil {
			return nil, errors.Trace(err)
		}
		cfg, err := g.ModelConfigService.ModelConfig(context.TODO())
		if err != nil {
			return nil, errors.Trace(err)
		}
		modelInfo, err := modelInfoService.GetModelInfo(context.TODO())
		if err != nil {
			return nil, errors.Trace(err)
		}
		return newBroker(context.TODO(), environs.OpenParams{
			ControllerUUID: modelInfo.ControllerUUID.String(),
			Cloud:          cloudSpec,
			Config:         cfg,
		}, environs.NoopCredentialInvalidator())
	}
}
