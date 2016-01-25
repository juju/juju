// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
)

const apiName = "Model"

// Facade provides access to a machine environment worker's view of the world.
type Facade struct {
	*common.EnvironWatcher
	*ToolsVersionUpdater
}

// NewFacade returns a new api client facade instance.
func NewFacade(caller base.APICaller) *Facade {
	facadeCaller := base.NewFacadeCaller(caller, apiName)
	return &Facade{
		EnvironWatcher:      common.NewEnvironWatcher(facadeCaller),
		ToolsVersionUpdater: NewToolsVersionUpdater(facadeCaller),
	}
}
