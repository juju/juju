// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	"github.com/juju/description/v11"

	jujucloud "github.com/juju/juju/cloud"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/uuid"
)

// newEphemeralProviderConfigGetter returns a new ephemeral provider config
// getter for the given model being imported. Data must be populated from
// the model description or the controller only.
//
// To be used in conjunction with scope.EphemeralProviderFactory() to call
// EphemeralProviderRunnerFromConfig[T] during import operation Setup calls.
func newEphemeralProviderConfigGetter(
	controllerUUID string,
	model description.Model,
	servicesGetter ProviderConfigServicesGetter,
) (providertracker.EphemeralProviderConfigGetter, error) {
	modelCfg, err := config.New(config.NoDefaults, model.Config())
	if err != nil {
		return nil, internalerrors.Errorf("model config: %w", err)
	}

	creds := model.CloudCredential()
	var namedCreds jujucloud.Credential
	if creds != nil {
		namedCreds = jujucloud.NewNamedCredential(creds.Name(), jujucloud.AuthType(creds.AuthType()), creds.Attributes(), false)
	}
	return &ephemeralProviderConfigGetter{
		cloudName:        model.Cloud(),
		cloudRegion:      model.CloudRegion(),
		cloudCredentials: &namedCreds,
		controllerUUID:   controllerUUID,
		modelConfig:      modelCfg,
		modelUUID:        coremodel.UUID(model.UUID()),
		modelType:        model.Type(),
		servicesGetter:   servicesGetter,
	}, nil
}

// ephemeralProviderConfigGetter implements the [providertracker.EphemeralProviderConfigGetter]
// interface for use during model import.
type ephemeralProviderConfigGetter struct {
	cloudCredentials *jujucloud.Credential
	cloudName        string
	cloudRegion      string
	controllerUUID   string
	modelConfig      *config.Config
	modelType        string
	modelUUID        coremodel.UUID

	servicesGetter ProviderConfigServicesGetter
}

// GetEphemeralProviderConfig returns the ephemeral provider config for the
// model being imported. This model is not yet active, thus cannot be found
// by the provider tracker during import.
func (p *ephemeralProviderConfigGetter) GetEphemeralProviderConfig(
	ctx context.Context,
) (providertracker.EphemeralProviderConfig, error) {
	domainServices, err := p.servicesGetter.ServicesForModel(ctx, p.modelUUID)
	if err != nil {
		return providertracker.EphemeralProviderConfig{}, internalerrors.Errorf("services for model: %w", err)
	}

	cloud, err := domainServices.Cloud().Cloud(ctx, p.cloudName)
	if err != nil {
		return providertracker.EphemeralProviderConfig{}, internalerrors.Errorf("cloud: %w", err)
	}

	cUUID, err := uuid.UUIDFromString(p.controllerUUID)
	if err != nil {
		return providertracker.EphemeralProviderConfig{}, internalerrors.Errorf("controller uuid from string: %w", err)
	}

	spec, err := cloudspec.MakeCloudSpec(*cloud, p.cloudRegion, p.cloudCredentials)
	if err != nil {
		return providertracker.EphemeralProviderConfig{}, internalerrors.Errorf("make cloud spec: %w", err)
	}

	return providertracker.EphemeralProviderConfig{
		CloudSpec:      spec,
		ControllerUUID: cUUID,
		ModelConfig:    p.modelConfig,
		ModelType:      coremodel.ModelType(p.modelType),
	}, nil
}

// ProviderConfigServicesGetter provides access to the services required
// an EmphemeralProvider. Only use ControllerDomainServices.
type ProviderConfigServicesGetter interface {
	ServicesForModel(ctx context.Context, modelUUID coremodel.UUID) (ProviderConfigServices, error)
}

// ProviderConfigServices provides access to the services necessary to
// create an emphemeral provider config.
type ProviderConfigServices interface {
	Cloud() CloudService
}

type getterShim struct {
	servicesGetter services.DomainServicesGetter
}

func (g getterShim) ServicesForModel(ctx context.Context, modelUUID coremodel.UUID) (ProviderConfigServices, error) {
	svcs, err := g.servicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return nil, internalerrors.Capture(internalerrors.Errorf("services for model %q: %w", modelUUID, err))
	}
	return &servicesShim{DomainServices: svcs}, nil
}

type servicesShim struct {
	services.DomainServices
}

func (s servicesShim) Cloud() CloudService {
	return s.DomainServices.Cloud()
}
