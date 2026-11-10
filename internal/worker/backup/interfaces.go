// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"context"
	"io"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
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

// MachineService provides the machine lookup used to record the controller
// machine's cloud instance ID in the archive metadata.
type MachineService interface {
	// GetMachineUUID returns the UUID for the given machine.
	GetMachineUUID(ctx context.Context, mName machine.Name) (machine.UUID, error)
	// GetInstanceID returns the cloud instance ID for the given machine.
	// It errors if the machine has not been provisioned with a cloud
	// instance ID; implementations never return an empty ID with a nil
	// error.
	GetInstanceID(ctx context.Context, machineUUID machine.UUID) (instance.Id, error)
}

// ModelExportDomainServices provides access to the model export service.
type ModelExportDomainServices interface {
	// Export returns the model export service.
	Export() ModelExportService
}

// ModelServicesForFunc returns the export services for a given model UUID.
type ModelServicesForFunc func(ctx context.Context, modelUUID coremodel.UUID) (ModelExportDomainServices, error)

// GetMachineInstanceIDFunc resolves the cloud instance ID of the machine
// hosting the controller, for recording in archive metadata. The machine
// service is scoped to the controller model. The implementation is chosen by
// manifold wiring: machine-hosted controllers resolve it from the machine
// service, while controllers without a machine, such as Kubernetes, report
// none.
type GetMachineInstanceIDFunc func(
	ctx context.Context,
	machineID string,
	machineService MachineService,
) (string, error)

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
