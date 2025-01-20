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

// ResourceClient provides the functionality for getting a resource file.
type ResourceClient interface {
	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store. The charm's revision, if any,
	// is ignored.
	GetResource(context.Context, ResourceRequest) (ResourceData, error)
}

// CharmHub represents methods required from a charmhub lient talking to the
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

// Downloader defines an API for downloading and storing charms.
type Downloader interface {
	// Download looks up the requested resource using the appropriate store,
	// downloads it to a temporary file and returns a ReadCloser that deletes the
	// temporary file on closure.
	//
	// The resulting resource is verified to have the right hash and size.
	//
	// Returns [ErrUnexpectedHash] if the hash of the downloaded resource does not
	// match the expected hash.
	// Returns [ErrUnexpectedSize] if the size of the downloaded resource does not
	// match the expected size.
	Download(ctx context.Context, url *url.URL, sha384 string, size int64) (io.ReadCloser, error)
}
