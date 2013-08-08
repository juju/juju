// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The machiner package implements the API interface
// used by the uniter worker.
package uniter

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/apiserver/common"
)

// UniterAPI implements the API used by the uniter worker.
type UniterAPI struct {
	*common.LifeGetter
	*common.StatusSetter
	*common.DeadEnsurer
	*common.AgentEntityWatcher

	st   *state.State
	auth common.Authorizer
}

// NewUniterAPI creates a new instance of the Uniter API.
func NewUniterAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UniterAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	getCanRead := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			// TODO(go1.1): method expression
			return authorizer.AuthOwner(tag)
		}, nil
	}
	return &UniterAPI{
		LifeGetter:         common.NewLifeGetter(st, getCanRead),
		StatusSetter:       common.NewStatusSetter(st, getCanRead),
		DeadEnsurer:        common.NewDeadEnsurer(st, getCanRead),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, getCanRead),
		st:                 st,
		auth:               authorizer,
	}, nil
}

// TODO(dimitern): Add the other needed API calls.
