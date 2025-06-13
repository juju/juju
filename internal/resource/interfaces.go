// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"io"

	coreapplication "github.com/juju/juju/core/application"
	corelogger "github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/resource"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/resource/charmhub"
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
	// is ignored.
	GetResource(charmhub.ResourceRequest) (charmhub.ResourceData, error)
}

type ApplicationService interface {
	// GetUnitUUID returns the UUID for the named unit.
	GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error)

	// GetApplicationIDByUnitName returns the application ID for the named unit.
	GetApplicationIDByUnitName(ctx context.Context, name coreunit.Name) (coreapplication.ID, error)

	// GetApplicationIDByName returns an application ID by application name. It
	// returns an error if the application can not be found by the name.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// GetCharmLocatorByApplicationName returns a CharmLocator by application name.
	// It returns an error if the charm can not be found by the name. This can also
	// be used as a cheap way to see if a charm exists without needing to load the
	// charm metadata.
	GetCharmLocatorByApplicationName(ctx context.Context, name string) (charm.CharmLocator, error)

	// GetApplicationCharmOrigin returns the charm origin for the specified
	// application name.
	GetApplicationCharmOrigin(ctx context.Context, name string) (application.CharmOrigin, error)
}

type ResourceService interface {
	// GetApplicationResourceID returns the UUID of the resource specified by natural key
	// of application and resource name.
	GetApplicationResourceID(ctx context.Context, args resource.GetApplicationResourceIDArgs) (coreresource.UUID, error)

	// GetResource returns the identified application resource.
	GetResource(ctx context.Context, resourceUUID coreresource.UUID) (coreresource.Resource, error)

	// OpenResource returns the details of and a reader for the resource.
	//
	// The following error types can be expected to be returned:
	//   - [resourceerrors.StoredResourceNotFound] if the specified resource is not
	//     in the resource store.
	OpenResource(ctx context.Context, resourceUUID coreresource.UUID) (coreresource.Resource, io.ReadCloser, error)

	// StoreResource adds the application resource to blob storage and updates the
	// metadata. It also sets the retrival information for the resource.
	StoreResource(ctx context.Context, args resource.StoreResourceArgs) error

	// SetUnitResource sets the unit as using the resource.
	SetUnitResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
		unitUUID coreunit.UUID,
	) error
}

// ResourceClientGetter gets a client for getting resources.
type ResourceClientGetter interface {
	// GetResourceClient returns a ResourceGetter.
	GetResourceClient(ctx context.Context, logger corelogger.Logger) (charmhub.ResourceClient, error)
}
