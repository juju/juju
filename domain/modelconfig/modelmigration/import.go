// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/description/v9"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/modelconfig/service"
	"github.com/juju/juju/domain/modelconfig/state"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(
	coordinator Coordinator, defaultsProvider service.ModelDefaultsProvider, logger logger.Logger,
) {
	coordinator.Add(&importOperation{
		defaultsProvider: defaultsProvider,
		logger:           logger,
	})
}

// ImportService represents the service methods needed for model config.
type ImportService interface {
	// SetModelConfig will remove any existing model config for the model and
	// replace with the new config provided. The new config will also be hydrated
	// with any model default attributes that have not been set on the config.
	SetModelConfig(
		ctx context.Context,
		cfg map[string]any,
	) error
}

type importOperation struct {
	modelmigration.BaseOperation

	logger           logger.Logger
	service          ImportService
	defaultsProvider service.ModelDefaultsProvider
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import model configuration"
}

// Setup the import operation, this will ensure the service is created
// and ready to be used.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	i.service = service.NewService(
		i.defaultsProvider,
		config.NoControllerAttributesValidator(),
		state.NewState(scope.ModelDB()))
	return nil
}

// Execute the import on the model config description.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	attrs := model.Config()

	// If we don't have any model config, then there is something seriously
	// wrong. In this case, we should return an error.
	if len(attrs) == 0 {
		return errors.Errorf("model config %w", coreerrors.NotValid)
	}

	// Models imported from older controllers may contain config attributes
	// which have since been removed from use. We filter these out by removing
	// any incoming attributes not in the default list.
	defaults, err := i.defaultsProvider.ModelDefaults(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	defaultAttrs := set.NewStrings()
	for k := range defaults {
		defaultAttrs.Add(k)
	}

	for k, v := range attrs {
		if !defaultAttrs.Contains(k) {
			i.logger.Debugf(ctx, "model config attribute %s=%v is removed on import", k, v)
			delete(attrs, k)
		}
	}

	if err := i.service.SetModelConfig(ctx, attrs); err != nil {
		return errors.Capture(err)
	}
	return nil
}
