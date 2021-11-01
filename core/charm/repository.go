// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"net/url"

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"gopkg.in/macaroon.v2"
)

// Repository describes an API for querying charm/bundle information and
// downloading them from a store.
type Repository interface {
	// GetDownloadURL returns a url from which a charm can be downloaded
	// based on the given charm url and charm origin.  A charm origin
	// updated with the ID and hash for the download is also returned.
	GetDownloadURL(*charm.URL, Origin, macaroon.Slice) (*url.URL, Origin, error)

	// DownloadCharm retrieves specified charm from the store and saves its
	// contents to the specified path.
	DownloadCharm(charmURL *charm.URL, requestedOrigin Origin, macaroons macaroon.Slice, archivePath string) (CharmArchive, Origin, error)

	// ResolveWithPreferredChannel verified that the charm with the requested
	// channel exists.  If no channel is specified, the latests, most stable is
	// is used. It returns a charm URL which includes the most current revision,
	// if none was provided, a charm origin, and a slice of series supported by
	// this charm.
	ResolveWithPreferredChannel(*charm.URL, Origin, macaroon.Slice) (*charm.URL, Origin, []string, error)

	// ListResources returns a list of resources associated with a given charm.
	ListResources(*charm.URL, Origin, macaroon.Slice) ([]charmresource.Resource, error)
}

// RepositoryFactory is a factory for charm Repositories.
type RepositoryFactory interface {
	GetCharmRepository(src Source) (Repository, error)
}

// CharmArchive provides information about a downloaded charm archive.
type CharmArchive interface {
	charm.Charm

	Version() string
	LXDProfile() *charm.LXDProfile
}
