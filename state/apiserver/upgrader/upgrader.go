// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

//import (
//    "launchpad.net/juju-core/state/apiserver/common"
//    "launchpad.net/juju-core/state/api/params"
//    )

// Upgrader provides access to the Upgrader API facade.
type Upgrader struct {
}

// New creates a new client-side Upgrader facade.
func New() *Upgrader {
	return &Upgrader{}
}
