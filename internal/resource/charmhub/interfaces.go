// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
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

// ResourceClient provides the functionality for getting a resource file.
type ResourceClient interface {
	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store. The charm's revision, if any,
	// is ignored.
	GetResource(context.Context, ResourceRequest) (ResourceData, error)
}

// CharmHub represents methods required from a charmhub client talking to the
// charmhub api used by the local CharmHubClient
type CharmHub interface {
	// Download retrieves the specified charm from the store and saves its
	// contents to the specified path. Read the path to get the contents of the
	// charm.
	Download(ctx context.Context, url *url.URL, path string, options ...charmhub.DownloadOption) (*charmhub.Digest, error)
	// Refresh gets the recommended revisions to install/refresh for the given
	// charms, including resource revisions.
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}
