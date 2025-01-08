// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"io"

	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/state"
)

// ResourceData represents the response from store about a request for
// resource bytes.
type ResourceData struct {
	// ReadCloser holds the bytes for the resource.
	io.ReadCloser

	// Resource holds the metadata for the resource.
	Resource charmresource.Resource
}

// ResourceRequest represents a request for information about a resource.
type ResourceRequest struct {
	// Channel is the channel from which to request the resource info.
	CharmID CharmID

	// Name is the name of the resource we're asking about.
	Name string

	// Revision is the specific revision of the resource we're asking about.
	Revision int
}

// CharmID represents the underlying charm for a given application. This
// includes both the URL and the origin.
type CharmID struct {

	// URL of the given charm, includes the reference name and a revision.
	// Old style charm URLs are also supported i.e. charmstore.
	URL *charm.URL

	// Origin holds the origin of a charm. This includes the source of the
	// charm, along with the revision and channel to identify where the charm
	// originated from.
	Origin state.CharmOrigin
}
