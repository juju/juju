// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interfaces

import (
	"context"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charm"
)

// Downloader defines an API for downloading and storing charms.
type Downloader interface {
	DownloadAndStore(ctx context.Context, charmURL *charm.URL, requestedOrigin corecharm.Origin, force bool) (corecharm.Origin, error)
}
