// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"io"
	"net/url"

	coreapplication "github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/resource"
	"github.com/juju/juju/environs/config"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
)

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// ResourceGetter provides the functionality for getting a resource file.
type ResourceGetter interface {
	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store. The charm's revision, if any,
	// is ignored. If the identified resource is not in the charm store
	// then errors.NotFound is returned.
	//
	// But if you write any code that assumes a NotFound error returned
	// from this method means that the resource was not found, you fail
	// basic logic.
	GetResource(ResourceRequest) (ResourceData, error)
}

type ApplicationService interface {
	// GetUnitUUID returns the UUID for the named unit.
	GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error)

	// GetApplicationIDByUnitName returns the application ID for the named unit.
	GetApplicationIDByUnitName(ctx context.Context, name coreunit.Name) (coreapplication.ID, error)

	// GetApplicationIDByName returns an application ID by application name. It
	// returns an error if the application can not be found by the name.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// GetCharmByApplicationID returns the charm for the specified application
	// ID.
	GetCharmByApplicationID(ctx context.Context, id coreapplication.ID) (internalcharm.Charm, domaincharm.CharmLocator, error)
}

type ResourceService interface {
	// GetResourceUUID returns the UUID of the resource specified by natural key
	// of application and resource name.
	GetResourceUUID(ctx context.Context, args resource.GetResourceUUIDArgs) (coreresource.UUID, error)

	// GetResource returns the identified application resource.
	GetResource(ctx context.Context, resourceUUID coreresource.UUID) (resource.Resource, error)

	// OpenResource returns the details of and a reader for the resource.
	//
	// The following error types can be expected to be returned:
	//   - [resourceerrors.StoredResourceNotFound] if the specified resource is not
	//     in the resource store.
	OpenResource(ctx context.Context, resourceUUID coreresource.UUID) (resource.Resource, io.ReadCloser, error)

	// StoreResource adds the application resource to blob storage and updates the
	// metadata. It also sets the retrival information for the resource.
	StoreResource(ctx context.Context, args resource.StoreResourceArgs) error

	// SetUnitResource sets the unit as using the resource.
	SetUnitResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
		unitUUID coreunit.UUID,
	) error

	// SetApplicationResource marks an existing resource as in use by a CAAS
	// application.
	SetApplicationResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
	) error
}

// CharmHub represents methods required from a charmhub client talking to the
// charmhub api used by the local CharmHubClient
type CharmHub interface {
	DownloadResource(ctx context.Context, resourceURL *url.URL) (r io.ReadCloser, err error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}
