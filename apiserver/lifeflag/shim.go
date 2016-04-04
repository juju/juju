// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade(
		"LifeFlag", 1,
		func(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*Facade, error) {
			return NewFacade(st, resources, authorizer)
		},
	)
}
