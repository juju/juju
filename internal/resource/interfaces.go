// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"io"
	"net/url"

	"github.com/juju/juju/core/resources"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	"github.com/juju/juju/state"
)

// Resources represents the methods used by the resource opener from state.Resources.
type Resources interface {
	// GetResource returns the identified resource.
	GetResource(applicationID, name string) (resources.Resource, error)
	// OpenResource returns the metadata for a resource and a reader for the resource.
	OpenResource(applicationID, name string) (resources.Resource, io.ReadCloser, error)
	// OpenResourceForUniter returns the metadata for a resource and a reader for the resource.
	OpenResourceForUniter(unitName, resName string) (resources.Resource, io.ReadCloser, error)
	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader, _ state.IncrementCharmModifiedVersionType) (resources.Resource, error)
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

// CharmHub represents methods required from a charmhub client talking to the
// charmhub api used by the local CharmHubClient
type CharmHub interface {
	DownloadResource(ctx context.Context, resourceURL *url.URL) (r io.ReadCloser, err error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}
