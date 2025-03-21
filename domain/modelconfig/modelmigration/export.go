// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/modelconfig/service"
	"github.com/juju/juju/domain/modelconfig/state"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator) {
	coordinator.Add(&exportOperation{})
}

// ExportService provides a subset of the external controller domain
// service methods needed for external controller export.
type ExportService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
}

// exportOperation describes a way to execute a migration for
// exporting external controllers.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export model configuration"
}

// Setup the export operation, this will ensure the service is created
// and ready to be used.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewService(
		// We shouldn't be using model defaults during export, so we use a
		// no-op provider.
		noopModelDefaultsProvider{},
		config.ModelValidator(),
		state.NewState(scope.ModelDB()))
	return nil
}

// Execute the migration of the model config to the description.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	config, err := e.service.ModelConfig(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// If the config is empty we don't want to export it, mark it as an error.
	// If we do export with an empty model config, then the import will fail,
	// which will put us in a worse state than when we started.
	if config == nil || len(config.AllAttrs()) == 0 {
		return errors.Errorf("empty model config %w", coreerrors.NotValid)
	}

	model.UpdateConfig(config.AllAttrs())

	return nil
}

type noopModelDefaultsProvider struct{}

// ModelDefaults will return the default config values to be used for a model
// and its config.
func (noopModelDefaultsProvider) ModelDefaults(context.Context) (modeldefaults.Defaults, error) {
	return modeldefaults.Defaults{}, errors.Errorf("model defaults %w", coreerrors.NotSupported)
}
