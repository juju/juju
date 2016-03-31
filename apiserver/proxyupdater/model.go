// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("ProxyUpdater", 1, NewAPI)
}

// NewAPI creates a new API server-side facade with a state.State backing.
func NewAPI(st *state.State, res *common.Resources, auth common.Authorizer) (API, error) {
	return newAPIWithBacking(&stateShim{st: st}, res, auth)
}
