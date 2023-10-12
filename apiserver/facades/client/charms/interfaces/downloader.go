// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interfaces

import (
	"context"

	"github.com/juju/charm/v11"

	corecharm "github.com/juju/juju/core/charm"
)

// Downloader defines an API for downloading and storing charms.
type Downloader interface {
	// DownloadAndStore downloads the charm at the specified URL and stores it
	// in the object store.
	DownloadAndStore(context.Context, *charm.URL, corecharm.Origin, bool) (corecharm.Origin, error)
}
