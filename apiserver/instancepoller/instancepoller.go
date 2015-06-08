// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"fmt"

	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("InstancePoller", 1, NewInstancePollerAPI)
}

var logger = loggo.GetLogger("juju.apiserver.instancepoller")

// InstancePollerAPI provides access to the InstancePoller API facade.
type InstancePollerAPI struct {
	*common.LifeGetter
	*common.EnvironWatcher
	*common.EnvironMachinesWatcher
	*common.InstanceIdGetter
	*common.StatusGetter

	st            StateInterface
	resources     *common.Resources
	authorizer    common.Authorizer
	accessMachine common.GetAuthFunc
}

// NewInstancePollerAPI creates a new server-side InstancePoller API
// facade.
func NewInstancePollerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*InstancePollerAPI, error) {

	if !authorizer.AuthEnvironManager() {
		// InstancePoller must run as environment manager.
		return nil, common.ErrPerm
	}
	accessMachine := common.AuthFuncForTagKind(names.MachineTagKind)
	sti := getState(st)

	// Life() is supported for machines.
	lifeGetter := common.NewLifeGetter(
		sti,
		accessMachine,
	)
	// EnvironConfig() and WatchForEnvironConfigChanges() are allowed
	// with unrestriced access.
	environWatcher := common.NewEnvironWatcher(
		sti,
		resources,
		authorizer,
	)
	// WatchEnvironMachines() is allowed with unrestricted access.
	machinesWatcher := common.NewEnvironMachinesWatcher(
		sti,
		resources,
		authorizer,
	)
	// InstanceId() is supported for machines.
	instanceIdGetter := common.NewInstanceIdGetter(
		sti,
		accessMachine,
	)
	// Status() is supported for machines.
	statusGetter := common.NewStatusGetter(
		sti,
		accessMachine,
	)

	return &InstancePollerAPI{
		LifeGetter:             lifeGetter,
		EnvironWatcher:         environWatcher,
		EnvironMachinesWatcher: machinesWatcher,
		InstanceIdGetter:       instanceIdGetter,
		StatusGetter:           statusGetter,
		st:                     sti,
		resources:              resources,
		authorizer:             authorizer,
		accessMachine:          accessMachine,
	}, nil
}

func (a *InstancePollerAPI) getOneMachine(tag string, canAccess common.AuthFunc) (StateMachine, error) {
	machineTag, err := names.ParseMachineTag(tag)
	if err != nil {
		return nil, err
	}
	if !canAccess(machineTag) {
		return nil, common.ErrPerm
	}
	entity, err := a.st.FindEntity(machineTag)
	if err != nil {
		return nil, err
	}
	machine, ok := entity.(StateMachine)
	if !ok {
		return nil, common.NotSupportedError(
			machineTag, fmt.Sprintf("expected machine, got %T", entity),
		)
	}
	return machine, nil
}

// ProviderAddresses returns the list of all known provider addresses
// for each given entity. Only machine tags are accepted.
func (a *InstancePollerAPI) ProviderAddresses(args params.Entities) (params.MachineAddressesResults, error) {
	result := params.MachineAddressesResults{
		Results: make([]params.MachineAddressesResult, len(args.Entities)),
	}
	canAccess, err := a.accessMachine()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err == nil {
			addrs := machine.ProviderAddresses()
			result.Results[i].Addresses = params.FromNetworkAddresses(addrs)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetProviderAddresses updates the list of known provider addresses
// for each given entity. Only machine tags are accepted.
func (a *InstancePollerAPI) SetProviderAddresses(args params.SetMachinesAddresses) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.MachineAddresses)),
	}
	canAccess, err := a.accessMachine()
	if err != nil {
		return result, err
	}
	for i, arg := range args.MachineAddresses {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err == nil {
			addrsToSet := params.NetworkAddresses(arg.Addresses)
			err = machine.SetProviderAddresses(addrsToSet...)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// InstanceStatus returns the instance status for each given entity.
// Only machine tags are accepted.
func (a *InstancePollerAPI) InstanceStatus(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := a.accessMachine()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err == nil {
			result.Results[i].Result, err = machine.InstanceStatus()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetInstanceStatus updates the instance status for each given
// entity. Only machine tags are accepted.
func (a *InstancePollerAPI) SetInstanceStatus(args params.SetInstancesStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := a.accessMachine()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err == nil {
			err = machine.SetInstanceStatus(arg.Status)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// AreManuallyProvisioned returns whether each given entity is
// manually provisioned or not. Only machine tags are accepted.
func (a *InstancePollerAPI) AreManuallyProvisioned(args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := a.accessMachine()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err == nil {
			result.Results[i].Result, err = machine.IsManual()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
