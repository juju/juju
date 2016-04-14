// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// NewAPI creates a new API server-side facade with a state.State backing.
func NewAPI(st *state.State, res *common.Resources, auth common.Authorizer) (*ProxyUpdaterAPI, error) {
	return NewAPIWithBacking(&stateShim{st: st}, res, auth)
}
