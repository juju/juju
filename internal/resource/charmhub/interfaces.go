// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"io"
	"net/url"

	"github.com/juju/juju/environs/config"
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

// CharmHub represents methods required from a charmhub client talking to the
// charmhub api used by the local CharmHubClient
type CharmHub interface {
	DownloadResource(ctx context.Context, resourceURL *url.URL) (r io.ReadCloser, err error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}
