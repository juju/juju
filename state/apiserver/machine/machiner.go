// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The machiner package implements the API interface
// used by the machiner worker.
package machine

import (
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

// MachinerAPI implements the API used by the machiner worker.
type MachinerAPI struct {
	*common.LifeGetter
	*common.StatusSetter
	*common.DeadEnsurer
	*common.AgentEntityWatcher

	st   *state.State
	auth common.Authorizer
}

// NewMachinerAPI creates a new instance of the Machiner API.
func NewMachinerAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*MachinerAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	// getAuthEnviron returns an AuthFunc that permits anyone
	// access to an environment tag.
	getAuthEnviron := func() (common.AuthFunc, error) {
		auth := func(tag string) bool {
			kind, err := names.TagKind(tag)
			if err != nil {
				panic(err)
			}
			return kind == names.EnvironTagKind
		}
		return auth, nil
	}
	getAuthOwner := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	getCanRead := common.AuthEither(getAuthEnviron, getAuthOwner)
	return &MachinerAPI{
		LifeGetter:         common.NewLifeGetter(st, getCanRead),
		StatusSetter:       common.NewStatusSetter(st, getAuthOwner),
		DeadEnsurer:        common.NewDeadEnsurer(st, getAuthOwner),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, getCanRead),
		st:                 st,
		auth:               authorizer,
	}, nil
}

// Environment returns the tag and life of the specified
// machine's enclosing environment.
func (m *MachinerAPI) Environment(args params.MachineEnvironment) (params.MachineEnvironmentResult, error) {
	// TODO(axw) when we have multi-tenancy, translate args.MachineTag to
	// the corresponding machine's environment.
	var result params.MachineEnvironmentResult
	env, err := m.st.Environment()
	if err != nil {
		return result, err
	}
	result.EnvironmentTag = env.Tag()
	result.Life = params.Life(env.Life().String())
	return result, nil
}
