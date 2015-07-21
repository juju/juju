// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
)

var logger = loggo.GetLogger("juju.api.space")

const spacesFacade = "Spaces"

// API provides access to the InstancePoller API facade.
type API struct {
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Spaces facade.
func NewAPI(caller base.APICaller) *API {
	if caller == nil {
		panic("caller is nil")
	}
	facadeCaller := base.NewFacadeCaller(caller, spacesFacade)
	return &API{
		//EnvironWatcher: common.NewEnvironWatcher(facadeCaller),
		facade: facadeCaller,
	}
}
