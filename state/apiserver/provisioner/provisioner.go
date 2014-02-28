// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/juju-core/constraints"
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
	*common.StateAddresser
	*common.APIAddresser
	*common.ToolsGetter
	*common.EnvironWatcher
	*common.EnvironMachinesWatcher
	*common.InstanceIdGetter

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
	// Both provisioner types can watch the environment.
	getCanWatch := common.AuthAlways(true)
	// Only the environment provisioner can read secrets.
	getCanReadSecrets := common.AuthAlways(authorizer.AuthEnvironManager())
	return &ProvisionerAPI{
		Remover:                common.NewRemover(st, false, getAuthFunc),
		StatusSetter:           common.NewStatusSetter(st, getAuthFunc),
		DeadEnsurer:            common.NewDeadEnsurer(st, getAuthFunc),
		PasswordChanger:        common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:             common.NewLifeGetter(st, getAuthFunc),
		StateAddresser:         common.NewStateAddresser(st),
		APIAddresser:           common.NewAPIAddresser(st),
		ToolsGetter:            common.NewToolsGetter(st, getAuthFunc),
		EnvironWatcher:         common.NewEnvironWatcher(st, resources, getCanWatch, getCanReadSecrets),
		EnvironMachinesWatcher: common.NewEnvironMachinesWatcher(st, resources, getCanReadSecrets),
		InstanceIdGetter:       common.NewInstanceIdGetter(st, getAuthFunc),
		st:                     st,
		resources:              resources,
		authorizer:             authorizer,
		getAuthFunc:            getAuthFunc,
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
	var watch state.StringsWatcher
	if arg.ContainerType != "" {
		watch = machine.WatchContainers(instance.ContainerType(arg.ContainerType))
	} else {
		watch = machine.WatchAllContainers()
	}
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: p.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.MustErr(watch)
}

// WatchContainers starts a StringsWatcher to watch containers deployed to
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

// WatchAllContainers starts a StringsWatcher to watch all containers deployed to
// any machine passed in args.
func (p *ProvisionerAPI) WatchAllContainers(args params.WatchContainers) (params.StringsWatchResults, error) {
	return p.WatchContainers(args)
}

// SetSupportedContainers updates the list of containers supported by the machines passed in args.
func (p *ProvisionerAPI) SetSupportedContainers(
	args params.MachineContainersParams) (params.ErrorResults, error) {

	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Params)),
	}
	for i, arg := range args.Params {
		canAccess, err := p.getAuthFunc()
		if err != nil {
			return result, err
		}
		machine, err := p.getMachine(canAccess, arg.MachineTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if len(arg.ContainerTypes) == 0 {
			err = machine.SupportsNoContainers()
		} else {
			err = machine.SetSupportedContainers(arg.ContainerTypes)
		}
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

// ContainerConfig returns information from the environment config that are
// needed for container cloud-init.
func (p *ProvisionerAPI) ContainerConfig() (params.ContainerConfig, error) {
	result := params.ContainerConfig{}
	config, err := p.st.EnvironConfig()
	if err != nil {
		return result, err
	}
	result.ProviderType = config.Type()
	result.AuthorizedKeys = config.AuthorizedKeys()
	result.SSLHostnameVerification = config.SSLHostnameVerification()
	result.Proxy = config.ProxySettings()
	result.AptProxy = config.AptProxySettings()
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
			r.Status, r.Info, _, err = machine.Status()
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
