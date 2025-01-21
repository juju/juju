// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"io"
	"net/http"

	coreapplication "github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
)

// ResourceServiceGetter is an interface for getting an ResourceService.
type ResourceServiceGetter interface {
	// Resource returns the model's resource service.
	Resource(*http.Request) (ResourceService, error)
}

// ResourceAndApplicationServiceGetter is an interface for getting resource and
// application service.
type ResourceAndApplicationServiceGetter interface {
	// Resource returns the model's resource service.
	Resource(*http.Request) (ResourceService, error)

	// Application returns the model's application service.
	Application(*http.Request) (ApplicationService, error)
}

type ResourceService interface {
	// GetResourceUUIDByApplicationAndResourceName returns the ID of the application
	// resource specified by natural key of application and resource Name.
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
	GetResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
	) (coreresource.Resource, error)

	// OpenResource returns the details of and a reader for the resource.
	//   - [resourceerrors.StoredResourceNotFound] if the specified resource is not
	//     in the resource store.
	OpenResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
	) (coreresource.Resource, io.ReadCloser, error)

	// StoreResource adds the application resource to blob storage and updates the
	// metadata. It also sets the retrival information for the resource.
	StoreResource(
		ctx context.Context,
		args resource.StoreResourceArgs,
	) error

	// StoreResourceAndIncrementCharmModifiedVersion adds the application resource to blob storage and updates the
	// metadata. It also sets the retrival information for the resource.
	StoreResourceAndIncrementCharmModifiedVersion(
		ctx context.Context,
		args resource.StoreResourceArgs,
	) error

	// GetApplicationResourceID returns the ID of the application resource
	// specified by natural key of application and resource Name.
	GetApplicationResourceID(
		ctx context.Context,
		args resource.GetApplicationResourceIDArgs,
	) (coreresource.UUID, error)

	// SetUnitResource sets the resource metadata for a specific unit.
	SetUnitResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
		unitUUID coreunit.UUID,
	) error
}

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// GetApplicationIDByName returns an application ID by application Name. It
	// returns an error if the application can not be found by the Name.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// GetApplicationIDByUnitName returns the application ID for the named unit.
	GetApplicationIDByUnitName(ctx context.Context, unitName coreunit.Name) (coreapplication.ID, error)

	// GetUnitUUID returns the UUID for the named unit.
	GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error)
}

type ResourceOpenerGetter interface {
	Opener(*http.Request, ...string) (coreresource.Opener, error)
}

type Validator interface {
	Validate(
		reader io.ReadCloser,
		expectedSHA384 string,
		expectedSize int64,
	) (io.ReadCloser, error)
}
