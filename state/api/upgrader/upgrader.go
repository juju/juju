// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"launchpad.net/juju-core/state/api/common"
)

// Upgrader provides access to the Upgrader API facade.
type Upgrader struct {
	caller common.Caller
}

// New creates a new client-side Upgrader facade.
func New(caller common.Caller) *Upgrader {
	return &Upgrader{caller}
}
