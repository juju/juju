// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"context"
	"io"
	"net/url"

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/names/v4"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/repositories"
	corestate "github.com/juju/juju/state"
)

// ResourceOpenerState represents methods from state required to implement
// a resource Opener.
type ResourceOpenerState interface {
	// required for csClientState
	Charm(*charm.URL) (*corestate.Charm, error)
	ControllerConfig() (controller.Config, error)

	// required for the chClientState
	Model() (Model, error)

	// required for NewResourceOpener and OpenResource
	Resources() (Resources, error)
	Unit(string) (Unit, error)
}

// Model represents model methods required to open a resource.
type Model interface {
	Config() (*config.Config, error)
}

// Unit represents unit methods required to open a resource.
type Unit interface {
	resource.Unit

	Application() (Application, error)
	Tag() names.Tag
}

// Application represents application methods required to open a resource.
type Application interface {
	CharmOrigin() *corestate.CharmOrigin
}

// Resources represents the methods used by resourceCache from state.Resources .
type Resources interface {
	// GetResource returns the identified resource.
	GetResource(applicationID, name string) (resource.Resource, error)
	// OpenResourceForUniter returns the metadata for a resource and a reader for the resource.
	OpenResourceForUniter(unit resource.Unit, name string) (resource.Resource, io.ReadCloser, error)
	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)
}

type ResourceRetryClientGetterFn func(st ResourceOpenerState) ResourceRetryClientGetter

type ResourceRetryClientGetter interface {
	NewClient() (*ResourceRetryClient, error)
}

// ResourceClient defines a set of functionality that a client
// needs to define to support resources.
type ResourceClient interface {
	GetResource(req repositories.ResourceRequest) (data charmstore.ResourceData, err error)
}

// CharmHub represents methods required from a charmhub client talking to the
// charmhub api used by the local CharmHubClient
type CharmHub interface {
	DownloadResource(ctx context.Context, resourceURL *url.URL) (r io.ReadCloser, err error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

type Logger interface {
	Tracef(string, ...interface{})
}
