// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/common"
)

const apiName = "Environment"

// Facade provides access to a machine environment worker's view of the world.
type Facade struct {
	*common.EnvironWatcher
}

// NewFacade returns a new api client facade instance.
func NewFacade(caller base.Caller) *Facade {
	return &Facade{
		EnvironWatcher: common.NewEnvironWatcher(apiName, caller),
	}
}
