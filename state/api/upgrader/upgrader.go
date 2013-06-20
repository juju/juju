// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import "launchpad.net/juju-core/state/api/common"

//import "launchpad.net/juju-core/state/api/params"

// Machiner provides access to the Upgrader API facade.
type Upgrader struct {
	caller common.Caller
}
