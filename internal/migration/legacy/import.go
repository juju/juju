// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package legacy

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v12"
	"github.com/juju/errors"

	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/modeldefaults"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
)

// ImportModel deserializes a legacy description model and imports it through
// the per-domain legacy migration operation list.
func ImportModel(
	ctx context.Context,
	bytes []byte,
	scope modelmigration.ScopeForModel,
	domainServices services.DomainServicesGetter,
	controllerUUID string,
	logger corelogger.Logger,
	clock clock.Clock,
) error {
	model, err := description.Deserialize(bytes)
	if err != nil {
		return errors.Trace(err)
	}

	configGetter, err := newEphemeralProviderConfigGetter(
		controllerUUID, model, getterShim{servicesGetter: domainServices},
	)
	if err != nil {
		return internalerrors.Errorf("creating ephemeral provider config getter: %w", err)
	}

	modelUUID := coremodel.UUID(model.UUID())

	// The domain services are not available during the import, until the model
	// is created and activated. The model defaults provider is used to provide
	// the model defaults during migration, so access is lazy.
	modelDefaultsProvider := modelDefaultsProvider{
		modelUUID:      modelUUID,
		servicesGetter: domainServices,
	}

	coordinator := modelmigration.NewCoordinator(logger)
	ImportOperations(coordinator, modelDefaultsProvider, configGetter, clock, logger)
	if err := coordinator.Perform(ctx, scope(modelUUID), model); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type modelDefaultsProvider struct {
	modelUUID      coremodel.UUID
	servicesGetter services.DomainServicesGetter
}

func (p modelDefaultsProvider) ModelDefaults(ctx context.Context) (modeldefaults.Defaults, error) {
	domainServices, err := p.servicesGetter.ServicesForModel(ctx, p.modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelDefaults := domainServices.ModelDefaults()
	fn := modelDefaults.ModelDefaultsProvider(p.modelUUID)
	return fn(ctx)
}
