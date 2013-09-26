// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
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
	*common.Addresser

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
			// All containers with the authenticated machine as a
			// parent are accessible by it.
			return isMachineAgent && names.MachineTag(parentId) == authEntityTag
		}, nil
	}
	return &ProvisionerAPI{
		Remover:         common.NewRemover(st, false, getAuthFunc),
		StatusSetter:    common.NewStatusSetter(st, getAuthFunc),
		DeadEnsurer:     common.NewDeadEnsurer(st, getAuthFunc),
		PasswordChanger: common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:      common.NewLifeGetter(st, getAuthFunc),
		Addresser:       common.NewAddresser(st),
		st:              st,
		resources:       resources,
		authorizer:      authorizer,
		getAuthFunc:     getAuthFunc,
	}, nil
}

func (p *ProvisionerAPI) getMachine(canAccess common.AuthFunc, tag string) (*state.Machine, error) {
	if !canAccess(tag) {
		return nil, common.ErrPerm
	}
	entity, err := p.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// machine.
	return entity.(*state.Machine), nil
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

// WatchForEnvironConfigChanges returns a NotifyWatcher to observe
// changes to the environment configuration.
func (p *ProvisionerAPI) WatchForEnvironConfigChanges() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	watch := p.st.WatchForEnvironConfigChanges()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = p.resources.Register(watch)
	} else {
		return result, watcher.MustErr(watch)
	}
	return result, nil
}

// EnvironConfig returns the current environment's configuration.
func (p *ProvisionerAPI) EnvironConfig() (params.ConfigResult, error) {
	result := params.ConfigResult{}
	config, err := p.st.EnvironConfig()
	if err != nil {
		return result, err
	}
	allAttrs := config.AllAttrs()
	if !p.authorizer.AuthEnvironManager() {
		// Mask out any secrets in the environment configuration
		// with values of the same type, so it'll pass validation.
		//
		// TODO(dimitern) 201309-26 bug #1231384
		// This needs to change so we won't return anything to
		// entities other than the environment manager, but the
		// provisioner code should be refactored first.
		env, err := environs.New(config)
		if err != nil {
			return result, err
		}
		secretAttrs, err := env.Provider().SecretAttrs(config)
		for k := range secretAttrs {
			allAttrs[k] = "not available"
		}
	}
	result.Config = allAttrs
	return result, nil
}

// Status returns the status of each given machine entity.
func (p *ProvisionerAPI) Status(args params.Entities) (params.StatusResults, error) {
	result := params.StatusResults{
		Results: make([]params.StatusResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		machine, err := p.getMachine(canAccess, entity.Tag)
		if err == nil {
			r := &result.Results[i]
			r.Status, r.Info, err = machine.Status()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Series returns the deployed series for each given machine entity.
func (p *ProvisionerAPI) Series(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		machine, err := p.getMachine(canAccess, entity.Tag)
		if err == nil {
			result.Results[i].Result = machine.Series()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Constraints returns the constraints for each given machine entity.
func (p *ProvisionerAPI) Constraints(args params.Entities) (params.ConstraintsResults, error) {
	result := params.ConstraintsResults{
		Results: make([]params.ConstraintsResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		machine, err := p.getMachine(canAccess, entity.Tag)
		if err == nil {
			var cons constraints.Value
			cons, err = machine.Constraints()
			if err == nil {
				result.Results[i].Constraints = cons
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetProvisioned sets the provider specific machine id, nonce and
// metadata for each given machine. Once set, the instance id cannot
// be changed.
func (p *ProvisionerAPI) SetProvisioned(args params.SetProvisioned) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Machines)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Machines {
		machine, err := p.getMachine(canAccess, arg.Tag)
		if err == nil {
			err = machine.SetProvisioned(arg.InstanceId, arg.Nonce, arg.Characteristics)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// InstanceId returns the provider specific instance id for each given
// machine or an CodeNotProvisioned error, if not set.
func (p *ProvisionerAPI) InstanceId(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		machine, err := p.getMachine(canAccess, entity.Tag)
		if err == nil {
			var instanceId instance.Id
			instanceId, err = machine.InstanceId()
			if err == nil {
				result.Results[i].Result = string(instanceId)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// WatchEnvironMachines returns a StringsWatcher that notifies of
// changes to the lifecycles of the machines (but not containers) in
// the current environment.
func (p *ProvisionerAPI) WatchEnvironMachines() (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	if !p.authorizer.AuthEnvironManager() {
		return result, common.ErrPerm
	}
	watch := p.st.WatchEnvironMachines()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		result.StringsWatcherId = p.resources.Register(watch)
		result.Changes = changes
	} else {
		err := watcher.MustErr(watch)
		return result, fmt.Errorf("cannot obtain initial environment machines: %v", err)
	}
	return result, nil
}
