// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"launchpad.net/juju-core/charm"
)

// Endpoint represents one endpoint of a relation. It is just a wrapper
// around charm.Relation. No API calls to the server-side are needed to
// support the interface needed by the uniter worker.
type Endpoint struct {
	charm.Relation
}
