// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"net/url"

	"github.com/juju/charm/v10"
	charmresource "github.com/juju/charm/v10/resource"
)

// Repository describes an API for querying charm/bundle information and
// downloading them from a store.
type Repository interface {
	// GetDownloadURL returns a url from which a charm can be downloaded
	// based on the given charm url and charm origin.  A charm origin
	// updated with the ID and hash for the download is also returned.
	GetDownloadURL(*charm.URL, Origin) (*url.URL, Origin, error)

	// DownloadCharm retrieves specified charm from the store and saves its
	// contents to the specified path.
	DownloadCharm(charmURL *charm.URL, requestedOrigin Origin, archivePath string) (CharmArchive, Origin, error)

	// ResolveWithPreferredChannel verified that the charm with the requested
	// channel exists.  If no channel is specified, the latests, most stable is
	// used. It returns a charm URL which includes the most current revision,
	// if none was provided, a charm origin, and a slice of series supported by
	// this charm.
	ResolveWithPreferredChannel(*charm.URL, Origin) (*charm.URL, Origin, []string, error)

	// GetEssentialMetadata resolves each provided MetadataRequest and
	// returns back a slice with the results. The results include the
	// minimum set of metadata that is required for deploying each charm.
	GetEssentialMetadata(...MetadataRequest) ([]EssentialMetadata, error)

	// ListResources returns a list of resources associated with a given charm.
	ListResources(*charm.URL, Origin) ([]charmresource.Resource, error)

	// ResolveResources looks at the provided repository and backend (already
	// downloaded) resources to determine which to use. Provided (uploaded) take
	// precedence. If charmhub has a newer resource than the back end, use that.
	ResolveResources(resources []charmresource.Resource, id CharmID) ([]charmresource.Resource, error)

	// ResolveForDeploy does the same thing as ResolveWithPreferredChannel
	// returning EssentialMetadata also. Resources are returned if a
	// charm revision was not provided in the CharmID.
	ResolveForDeploy(CharmID) (ResolvedDataForDeploy, error)
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

// MetadataRequest encapsulates the arguments for a charm essential metadata
// resolution request.
type MetadataRequest struct {
	CharmURL *charm.URL
	Origin   Origin
}

// EssentialMetadata encapsulates the essential metadata required for deploying
// a particular charm.
type EssentialMetadata struct {
	ResolvedOrigin Origin

	Meta     *charm.Meta
	Manifest *charm.Manifest
	Config   *charm.Config
}

// CharmID encapsulates data for identifying a unique charm in a charm repository.
type CharmID struct {
	// URL is the url of the charm.
	URL *charm.URL

	// Origin holds the original source of a charm, including its channel.
	Origin Origin

	// Metadata is optional extra information about a particular model's
	// "in-theatre" use of the charm.
	Metadata map[string]string
}

// ResolvedDataForDeploy is the response data from ResolveForDeploy
type ResolvedDataForDeploy struct {
	URL *charm.URL

	EssentialMetadata EssentialMetadata

	// Resources is a map of resource names to their current repository revision
	// based on the supplied origin
	Resources map[string]charmresource.Resource
}
