// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package hostkeyreporter

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// newFacade wraps New to express the supplied *state.State as a Backend.
func newFacade(st *state.State, res *common.Resources, auth common.Authorizer) (*Facade, error) {
	return New(st, res, auth)
}
