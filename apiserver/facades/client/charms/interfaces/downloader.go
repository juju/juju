// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interfaces

import (
	"github.com/juju/charm/v9"
	"gopkg.in/macaroon.v2"

	corecharm "github.com/juju/juju/core/charm"
)

// Downloader defines an API for downloading and storing charms.
type Downloader interface {
	DownloadAndStore(charmURL *charm.URL, requestedOrigin corecharm.Origin, macaroons macaroon.Slice, force bool) (corecharm.Origin, error)
}
