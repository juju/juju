// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interfaces

import (
	"gopkg.in/macaroon.v2"

	"github.com/juju/charm/v8"
	corecharm "github.com/juju/juju/v2/core/charm"
)

// Downloader defines an API for downloading and storing charms.
type Downloader interface {
	DownloadAndStore(charmURL *charm.URL, requestedOrigin corecharm.Origin, macaroons macaroon.Slice, force bool) (corecharm.Origin, error)
}
