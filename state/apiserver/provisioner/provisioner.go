// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
)

// ProvisionerAPI provides access to the Provisioner API facade.
type ProvisionerAPI struct {
	*common.Remover
	*common.StatusSetter
	*common.DeadEnsurer
	*common.PasswordChanger
	*common.LifeGetter

	st          *state.State
	resources   *common.Resources
	authorizer  common.Authorizer
	getAuthFunc common.GetAuthFunc
}

// NewProvisionerAPI creates a new server-side ProvisionerAPI facade.
func NewProvisionerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*ProvisionerAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}
	getAuthFunc := func() (common.AuthFunc, error) {
		isEnvironManager := authorizer.AuthEnvironManager()
		isMachineAgent := authorizer.AuthMachineAgent()
		authEntityTag := authorizer.GetAuthTag()

		return func(tag string) bool {
			if isMachineAgent && tag == authEntityTag {
				// A machine agent can always access its own machine.
				return true
			}
			_, id, err := names.ParseTag(tag, names.MachineTagKind)
			if err != nil {
				return false
			}
			parentId := state.ParentId(id)
			if parentId == "" {
				// All top-level machines are accessible by the
				// environment manager.
				return isEnvironManager
			}
			if names.MachineTag(parentId) == authEntityTag {
				// All containers with the authenticated machine as a
				// parent are accessible by it.
				return isMachineAgent
			}
			return false
		}, nil
	}
	return &ProvisionerAPI{
		Remover:         common.NewRemover(st, false, getAuthFunc),
		StatusSetter:    common.NewStatusSetter(st, getAuthFunc),
		DeadEnsurer:     common.NewDeadEnsurer(st, getAuthFunc),
		PasswordChanger: common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:      common.NewLifeGetter(st, getAuthFunc),
		st:              st,
		resources:       resources,
		authorizer:      authorizer,
		getAuthFunc:     getAuthFunc,
	}, nil
}

func (p *ProvisionerAPI) watchOneMachineContainers(arg params.WatchContainer) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return nothing, err
	}
	if !canAccess(arg.MachineTag) {
		return nothing, common.ErrPerm
	}
	_, id, err := names.ParseTag(arg.MachineTag, names.MachineTagKind)
	if err != nil {
		return nothing, err
	}
	machine, err := p.st.Machine(id)
	if err != nil {
		return nothing, err
	}
	watch := machine.WatchContainers(instance.ContainerType(arg.ContainerType))
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: p.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.MustErr(watch)
}

// WatchContainers starts a StringsWatcher to watch all containers deployed to
// any machine passed in args.
func (p *ProvisionerAPI) WatchContainers(args params.WatchContainers) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Params)),
	}
	for i, arg := range args.Params {
		watcherResult, err := p.watchOneMachineContainers(arg)
		result.Results[i] = watcherResult
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// TODO(dimitern): Add methods to implement the followin at the client-side API:
// machine.Series()
// machine.Status()
// machine.Constraints()
// machine.SetProvisioned(inst.Id(), nonce, metadata)
// machine.InstanceId()
// p.st.WatchEnvironConfig()
// p.st.WatchEnvironMachines()
// st.EnvironConfig() (for worker/environ.go:WaitForEnviron)
