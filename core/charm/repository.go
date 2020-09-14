// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"net/url"

	"github.com/juju/charm/v8"
	"github.com/juju/juju/apiserver/params"
)

// Repository represents the necessary methods to resolve and download
// charms from a repository where they reside.
type Repository interface {
	// FindDownloadURL returns a url from which a charm can be downloaded
	// based on the given charm url and charm origin.  A charm origin
	// updated with the ID and hash for the download is also returned.
	FindDownloadURL(curl *charm.URL, origin Origin) (*url.URL, Origin, error)

	// DownloadCharm reads the charm referenced by curl or downloadURL into
	// a file with the given path, which will be created if needed. Note
	// that the path's parent directory must already exist.
	DownloadCharm(curl *charm.URL, downloadURL *url.URL, archivePath string) (*charm.CharmArchive, error)

	// ResolveWithPreferredChannel verified that the charm with the requested
	// channel exists.  If no channel is specified, the latests, most stable is
	// is used. It returns a charm URL which includes the most current revision,
	// if none was provided, a charm origin, and a slice of series supported by
	// this charm.
	ResolveWithPreferredChannel(*charm.URL, params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error)
}
