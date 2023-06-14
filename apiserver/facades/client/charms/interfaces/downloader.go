// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interfaces

import (
	"github.com/juju/charm/v11"

	corecharm "github.com/juju/juju/core/charm"
)

// Downloader defines an API for downloading and storing charms.
type Downloader interface {
	DownloadAndStore(charmURL *charm.URL, requestedOrigin corecharm.Origin, force bool) (corecharm.Origin, error)
}
