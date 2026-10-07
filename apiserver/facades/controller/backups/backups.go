// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	domainexport "github.com/juju/juju/domain/export"
	environsconfig "github.com/juju/juju/environs/config"
)

// ControllerExportService exports the controller database.
type ControllerExportService interface {
	// Export exports all controller data.
	Export(ctx context.Context) (*domainexport.ControllerExport, error)
}

// ModelExportService describes the ability to acquire a model export.
type ModelExportService interface {
	// Export returns a complete representation of the model database.
	Export(ctx context.Context) (*domainexport.ModelExport, error)
}

// ModelExportDomainServices provides access to the model export service.
// It is satisfied by [services.DomainServices].
type ModelExportDomainServices interface {
	// Export returns the model export service.
	Export() ModelExportService
}

// ModelServicesForFunc returns the export services for a given model UUID.
type ModelServicesForFunc func(ctx context.Context, modelUUID coremodel.UUID) (ModelExportDomainServices, error)

// ModelConfigService provides the model configuration, used to resolve the
// backup directory.
type ModelConfigService interface {
	// ModelConfig returns the model config.
	ModelConfig(ctx context.Context) (*environsconfig.Config, error)
}

// ControllerModelLister lists the model namespaces registered in the
// controller database.
type ControllerModelLister interface {
	// GetModelNamespaces returns the UUIDs of models registered in the
	// controller database.
	GetModelNamespaces(ctx context.Context) ([]string, error)
}

// ControllerNodeLister lists the controller machine IDs making up the
// controller quorum.
type ControllerNodeLister interface {
	// GetControllerIDs returns the list of controller machine IDs.
	GetControllerIDs(ctx context.Context) ([]string, error)
}

// API provides backup-specific API methods.
type API struct {
	authorizer     facade.Authorizer
	controllerUUID string
	logger         corelogger.Logger
}

// NewAPI creates a new instance of the Backups API facade.
func NewAPI(
	authorizer facade.Authorizer,
	controllerUUID string,
	logger corelogger.Logger,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &API{
		authorizer:     authorizer,
		controllerUUID: controllerUUID,
		logger:         logger,
	}, nil
}
