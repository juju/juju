// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"github.com/juju/juju/api/base"
)

const apiName = "AgentTools"

// Facade provides access to an api used for manipulating agent tools.
type Facade struct {
	facade base.FacadeCaller
}

// NewFacade returns a new api client facade instance.
func NewFacade(caller base.APICaller) *Facade {
	facadeCaller := base.NewFacadeCaller(caller, apiName)
	return &Facade{facadeCaller}
}

// UpdateToolsVersion calls UpdateToolsAvailable in the server.
func (f *Facade) UpdateToolsVersion() error {
	return f.facade.FacadeCall("UpdateToolsAvailable", nil, nil)
}
