// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"io"
	"net/http"

	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
)

// ResourceService defines operations related to managing application resources.
type ResourceService interface {
	// GetResourceUUIDByApplicationAndResourceName returns the ID of the
	// application resource specified by natural key of application and resource
	// Name.
	// The following error types can be expected to be returned:
	//   - [resourceerrors.ResourceNotFound] if the specified resource does not
	//     exist.
	//   - [resourceerrors.ApplicationNotFound] if the specified application
	//     does not exist.
	GetResourceUUIDByApplicationAndResourceName(ctx context.Context, appName string, resName string) (coreresource.UUID, error)

	// GetResource returns the identified application resource.
	// The following error types can be expected to be returned:
	//   - [resourceerrors.ResourceNotFound] if the specified resource does not
	//     exist.
	GetResource(ctx context.Context, resourceUUID coreresource.UUID) (coreresource.Resource, error)

	// OpenResource returns the details of and a reader for the resource.
	// The following error types can be expected to be returned:
	//   - [resourceerrors.ResourceNotFound] if the specified resource does not
	//     exist.
	//   - [resourceerrors.StoredResourceNotFound] if the specified resource is
	//     not in the resource store.
	OpenResource(ctx context.Context, resourceUUID coreresource.UUID) (coreresource.Resource, io.ReadCloser, error)

	// StoreResource adds the application resource to blob storage and updates
	// the metadata. It also sets the retrival information for the resource.
	StoreResource(ctx context.Context, args resource.StoreResourceArgs) error

	// StoreResourceAndIncrementCharmModifiedVersion adds the application
	// resource to blob storage and updates the metadata. It also sets the
	// retrieval information for the resource.
	StoreResourceAndIncrementCharmModifiedVersion(ctx context.Context, args resource.StoreResourceArgs) error

	// GetApplicationResourceID returns the ID of the application resource
	// specified by the application and resource name.
	GetApplicationResourceID(ctx context.Context, args resource.GetApplicationResourceIDArgs) (coreresource.UUID, error)

	// SetUnitResource records that the unit is using the resource.
	SetUnitResource(ctx context.Context, resourceUUID coreresource.UUID, unitUUID coreunit.UUID) error

	// UpdateUploadResource adds a new entry for an uploaded blob in the resource
	// table with the desired parameters and sets it on the application. Any previous
	// resource blob is removed. The new resource UUID is returned.
	UpdateUploadResource(ctx context.Context, resourceToUpdate coreresource.UUID) (coreresource.UUID, error)
}

// ResourceServiceGetter is an interface for retrieving a ResourceService
// instance.
type ResourceServiceGetter interface {
	// Resource retrieves a ResourceService for handling resource-related
	// operations.
	Resource(*http.Request) (ResourceService, error)
}
