// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
)

var logger = loggo.GetLogger("juju.api.subnets")

const subnetsFacade = "Subnets"

// API provides access to the InstancePoller API facade.
type API struct {
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Subnets facade.
func NewAPI(caller base.APICaller) *API {
	if caller == nil {
		panic("caller is nil")
	}
	facadeCaller := base.NewFacadeCaller(caller, subnetsFacade)
	return &API{
		facade: facadeCaller,
	}
}
