// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

// CharmInfo holds the information about a charm from the charm store.
// The info relates to the charm at a particular revision at the time
// the charm store handled the request. The resource revisions
// associated with the charm at that revision may change at any time.
// Note, however, that the set of resource names remains fixed for any
// given charm revision.
type CharmInfo struct {
	// URL is the charm's URL, including its revision.
	URL *charm.URL

	// Resources is the list of resource info for each of the charm's
	// resources. This list is accurate as of the time that the
	// charm store handled the request for the charm info.
	Resources []charmresource.Resource
}

// CharmInfoResult holds the result of a charm store request for info
// about a charm.
type CharmInfoResult struct {
	CharmInfo

	// Error indicates a problem retrieving or processing the info
	// for this charm.
	Error error
}
