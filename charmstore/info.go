// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"time"

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

	// Timestamp indicates when the info came from the charm store.
	Timestamp time.Time

	// LatestRevision identifies the most recent revision of the charm
	// that is available in the charm store.
	LatestRevision int

	// LatestResources is the list of resource info for each of the
	// charm's resources. This list is accurate as of the time that the
	// charm store handled the request for the charm info.
	LatestResources []charmresource.Resource
}

// LatestURL returns the charm URL for the latest revision of the charm.
func (info CharmInfo) LatestURL() *charm.URL {
	return info.URL.WithRevision(info.LatestRevision)
}

// CharmInfoResult holds the result of a charm store request for info
// about a charm.
type CharmInfoResult struct {
	CharmInfo

	// Error indicates a problem retrieving or processing the info
	// for this charm.
	Error error
}
