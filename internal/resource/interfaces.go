// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"io"

	"github.com/juju/names/v5"

	coreapplication "github.com/juju/juju/core/application"
	corelogger "github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/resource"
	"github.com/juju/juju/environs/config"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/resource/charmhub"
	"github.com/juju/juju/state"
)

type DeprecatedState interface {
	Unit(name string) (DeprecatedStateUnit, error)
	Application(name string) (DeprecatedStateApplication, error)
	ModelUUID() string
}

type DeprecatedStateUnit interface {
	ApplicationName() string
	CharmURL() *string
	Tag() names.Tag
}

type DeprecatedStateApplication interface {
	CharmOrigin() *state.CharmOrigin
	CharmURL() (*string, bool)
	Tag() names.Tag
}

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

	// GetCharmByApplicationID returns the charm for the specified application
	// ID.
	GetCharmByApplicationID(ctx context.Context, id coreapplication.ID) (internalcharm.Charm, domaincharm.CharmLocator, error)
}

type ResourceService interface {
	// GetApplicationResourceID returns the UUID of the resource specified by natural key
	// of application and resource name.
	GetApplicationResourceID(ctx context.Context, args resource.GetApplicationResourceIDArgs) (coreresource.UUID, error)

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

// ResourceClientGetter gets a client for getting resources.
type ResourceClientGetter interface {
	// GetResourceClient returns a ResourceGetter.
	GetResourceClient(ctx context.Context, logger corelogger.Logger) (charmhub.ResourceGetter, error)
}
