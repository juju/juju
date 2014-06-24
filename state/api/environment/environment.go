// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/common"
)

const apiName = "Environment"

// Facade provides access to a machine environment worker's view of the world.
type Facade struct {
	*common.EnvironWatcher
}

// NewFacade returns a new api client facade instance.
func NewFacade(caller base.APICaller) *Facade {
	return &Facade{
		EnvironWatcher: common.NewEnvironWatcher(apiName, caller),
	}
}
