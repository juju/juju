// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"io"
	"net/http"

	coreapplication "github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
)

// ApplicationService defines operations related to managing applications.
type ApplicationService interface {
	// GetApplicationIDByName returns an application ID by application name. It
	// returns an error if the application can not be found by the name.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// GetApplicationIDByUnitName returns the application ID for the named unit.
	GetApplicationIDByUnitName(ctx context.Context, unitName coreunit.Name) (coreapplication.ID, error)

	// GetUnitUUID returns the UUID for the named unit.
	GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error)
}

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
	GetResourceUUIDByApplicationAndResourceName(
		ctx context.Context,
		appName string,
		resName string,
	) (coreresource.UUID, error)

	// GetResource returns the identified application resource.
	// The following error types can be expected to be returned:
	//   - [resourceerrors.ResourceNotFound] if the specified resource does not
	//     exist.
	GetResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
	) (coreresource.Resource, error)

	// OpenResource returns the details of and a reader for the resource.
	// The following error types can be expected to be returned:
	//   - [resourceerrors.ResourceNotFound] if the specified resource does not
	//     exist.
	//   - [resourceerrors.StoredResourceNotFound] if the specified resource is
	//     not in the resource store.
	OpenResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
	) (coreresource.Resource, io.ReadCloser, error)

	// StoreResource adds the application resource to blob storage and updates
	// the metadata. It also sets the retrival information for the resource.
	StoreResource(
		ctx context.Context,
		args resource.StoreResourceArgs,
	) error

	// StoreResourceAndIncrementCharmModifiedVersion adds the application
	// resource to blob storage and updates the metadata. It also sets the
	// retrival information for the resource.
	StoreResourceAndIncrementCharmModifiedVersion(
		ctx context.Context,
		args resource.StoreResourceArgs,
	) error

	// GetApplicationResourceID returns the ID of the application resource
	// specified by the application and resource name.
	GetApplicationResourceID(
		ctx context.Context,
		args resource.GetApplicationResourceIDArgs,
	) (coreresource.UUID, error)

	// SetUnitResource records that the unit is using the resource.
	SetUnitResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
		unitUUID coreunit.UUID,
	) error
}

// ResourceServiceGetter is an interface for retrieving a ResourceService
// instance.
type ResourceServiceGetter interface {
	// Resource retrieves a ResourceService for handling resource-related
	// operations.
	Resource(*http.Request) (ResourceService, error)
}

// ApplicationServiceGetter is an interface for getting an ApplicationService.
type ApplicationServiceGetter interface {
	// Application returns the model's application service.
	Application(*http.Request) (ApplicationService, error)
}
