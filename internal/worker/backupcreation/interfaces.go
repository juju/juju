// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupcreation

import (
	"context"
	"io"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
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

// ReadObjectStore is the subset of the object store used to stream
// objects into the backup archive.
type ReadObjectStore interface {
	// GetBySHA256Prefix opens an object whose SHA-256 starts with the given
	// prefix, retrieving missing local blobs from other controller nodes.
	// The caller must close the returned reader.
	GetBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, objectstore.Digest, error)
}

// ObjectStoreForModelFunc returns the object store for a given model
// UUID.
type ObjectStoreForModelFunc func(ctx context.Context, modelUUID string) (ReadObjectStore, error)
