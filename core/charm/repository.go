// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"net/url"

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
)

// Repository represents the necessary methods to resolve and download
// charms from a repository where they reside.
type Repository interface {
	// FindDownloadURL returns a url from which a charm can be downloaded
	// based on the given charm url and charm origin.  A charm origin
	// updated with the ID and hash for the download is also returned.
	FindDownloadURL(*charm.URL, Origin) (*url.URL, Origin, error)

	// DownloadCharm reads the charm referenced the resource URL or downloads
	// into a file with the given path, which will be created if needed.
	// It is expected that the URL for charm store will be in the correct
	// form i.e that it parses to a charm.URL.
	DownloadCharm(resourceURL, archivePath string) (*charm.CharmArchive, error)

	// ResolveWithPreferredChannel verified that the charm with the requested
	// channel exists.  If no channel is specified, the latests, most stable is
	// is used. It returns a charm URL which includes the most current revision,
	// if none was provided, a charm origin, and a slice of series supported by
	// this charm.
	ResolveWithPreferredChannel(*charm.URL, Origin) (*charm.URL, Origin, []string, error)

	// ListResources returns a list of resources associated with a given charm.
	ListResources(*charm.URL, Origin) ([]charmresource.Resource, error)
}
