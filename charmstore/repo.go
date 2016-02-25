// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
)

// Repo exposes the functionality Juju needs from charmreo.CharmStore.
type Repo interface {
	charmrepo.Interface

	// Latest returns the most up-to-date information about each of the
	// identified charms at their latest revision. The revisions in the
	// provided URLs are ignored.
	Latest(...*charm.URL) ([]charmrepo.CharmRevision, error)
}
