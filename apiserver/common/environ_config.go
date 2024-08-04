// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"

	"github.com/juju/names/v5"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// EnvironConfigGetterFuncs holds implements environs.EnvironConfigGetter
// in a pluggable way.
type EnvironConfigGetterFuncs struct {
	ModelConfigFunc func() (*config.Config, error)
	CloudSpecFunc   func() (environscloudspec.CloudSpec, error)
}

// ModelConfig implements EnvironConfigGetter.
func (f EnvironConfigGetterFuncs) ModelConfig(ctx context.Context) (*config.Config, error) {
	return f.ModelConfigFunc()
}

// CloudSpec implements environs.EnvironConfigGetter.
func (f EnvironConfigGetterFuncs) CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error) {
	return f.CloudSpecFunc()
}

// NewEnvironFunc is a function that returns a BootstrapEnviron instance.
type NewEnvironFunc func(context.Context) (environs.BootstrapEnviron, error)

// EnvironFuncForModel is a helper function that returns a NewEnvironFunc suitable for
// the specified model.
func EnvironFuncForModel(model stateenvirons.Model, cloudService CloudService,
	credentialService stateenvirons.CredentialService,
	configGetter environs.EnvironConfigGetter,
) NewEnvironFunc {
	if model.Type() == state.ModelTypeCAAS {
		return func(ctx context.Context) (environs.BootstrapEnviron, error) {
			f := stateenvirons.GetNewCAASBrokerFunc(caas.New)
			return f(model, cloudService, credentialService)
		}
	}
	return func(ctx context.Context) (environs.BootstrapEnviron, error) {
		return environs.GetEnviron(ctx, configGetter, environs.New)
	}
}

// ModelInfoService provides access to information about the model.
type ModelInfoService interface {
	// GetModelInfo returns read-only model information for the current model.
	GetModelInfo(context.Context) (model.ReadOnlyModel, error)
}

// ServiceEnvironConfigGetter implements environs.EnvironConfigGetter using
// domain services instead of Mongo state.
type ServiceEnvironConfigGetter struct {
	modelConfigService ModelConfigService
	modelInfoService   ModelInfoService
	cloudService       CloudService
	credentialService  CredentialService
}

func NewServiceEnvironConfigGetter(
	modelConfigService ModelConfigService,
	modelInfoService ModelInfoService,
	cloudService CloudService,
	credentialService CredentialService,
) *ServiceEnvironConfigGetter {
	return &ServiceEnvironConfigGetter{
		modelConfigService: modelConfigService,
		modelInfoService:   modelInfoService,
		cloudService:       cloudService,
		credentialService:  credentialService,
	}
}

func (s *ServiceEnvironConfigGetter) ModelConfig(ctx context.Context) (*config.Config, error) {
	return s.modelConfigService.ModelConfig(ctx)
}

func (s *ServiceEnvironConfigGetter) CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error) {
	// Get information for current model
	modelInfo, err := s.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return environscloudspec.CloudSpec{}, fmt.Errorf("getting info for current model: %w", err)
	}

	// Get the cloud of the current model
	cld, err := s.cloudService.Cloud(ctx, modelInfo.Cloud)
	if err != nil {
		return environscloudspec.CloudSpec{}, fmt.Errorf("getting cloud %q for model %q: %w",
			modelInfo.Cloud, modelInfo.UUID, err)
	}

	// Get the credential for the current model
	credentialTag := names.NewCloudCredentialTag(modelInfo.CredentialName)
	cred, err := s.credentialService.CloudCredential(ctx, credential.KeyFromTag(credentialTag))
	if err != nil {
		return environscloudspec.CloudSpec{}, fmt.Errorf("getting credential %q for model %q: %w",
			modelInfo.CredentialName, modelInfo.UUID, err)
	}

	return environscloudspec.MakeCloudSpec(*cld, modelInfo.CloudRegion, &cred)
}

var _ environs.EnvironConfigGetter = &ServiceEnvironConfigGetter{}
