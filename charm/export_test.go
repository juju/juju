// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"launchpad.net/juju-core/schema"
)

// Export meaningful bits for tests only.

func IfaceExpander(limit interface{}) schema.Checker {
	return ifaceExpander(limit)
}

func NewStore(url string) *CharmStore {
	return &CharmStore{url}
}
