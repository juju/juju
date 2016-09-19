// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The machiner package implements the API interface
// used by the machiner worker.
package machine

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/stateenvirons"
)

var logger = loggo.GetLogger("juju.apiserver.machine")

func init() {
	common.RegisterStandardFacade("Machiner", 1, NewMachinerAPI)
}

// MachinerAPI implements the API used by the machiner worker.
type MachinerAPI struct {
	*common.LifeGetter
	*common.StatusSetter
	*common.DeadEnsurer
	*common.AgentEntityWatcher
	*common.APIAddresser

	st           *state.State
	auth         facade.Authorizer
	getCanModify common.GetAuthFunc
	getCanRead   common.GetAuthFunc
}

// NewMachinerAPI creates a new instance of the Machiner API.
func NewMachinerAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*MachinerAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	getCanModify := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	getCanRead := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &MachinerAPI{
		LifeGetter:         common.NewLifeGetter(st, getCanRead),
		StatusSetter:       common.NewStatusSetter(st, getCanModify),
		DeadEnsurer:        common.NewDeadEnsurer(st, getCanModify),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, getCanRead),
		APIAddresser:       common.NewAPIAddresser(st, resources),
		st:                 st,
		auth:               authorizer,
		getCanModify:       getCanModify,
		getCanRead:         getCanRead,
	}, nil
}

func (api *MachinerAPI) getMachine(tag names.Tag) (*state.Machine, error) {
	entity, err := api.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	return entity.(*state.Machine), nil
}

func (api *MachinerAPI) SetMachineAddresses(args params.SetMachinesAddresses) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.MachineAddresses)),
	}
	canModify, err := api.getCanModify()
	if err != nil {
		return results, err
	}
	for i, arg := range args.MachineAddresses {
		tag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canModify(tag) {
			var m *state.Machine
			m, err = api.getMachine(tag)
			if err == nil {
				addresses := params.NetworkAddresses(arg.Addresses...)
				err = m.SetMachineAddresses(addresses...)
			} else if errors.IsNotFound(err) {
				err = common.ErrPerm
			}
		}
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

// Jobs returns the jobs assigned to the given entities.
func (api *MachinerAPI) Jobs(args params.Entities) (params.JobsResults, error) {
	result := params.JobsResults{
		Results: make([]params.JobsResult, len(args.Entities)),
	}

	canRead, err := api.getCanRead()
	if err != nil {
		return result, err
	}

	for i, agent := range args.Entities {
		tag, err := names.ParseMachineTag(agent.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		if !canRead(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		machine, err := api.getMachine(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		machineJobs := machine.Jobs()
		jobs := make([]multiwatcher.MachineJob, len(machineJobs))
		for i, job := range machineJobs {
			jobs[i] = job.ToParams()
		}
		result.Results[i].Jobs = jobs
	}
	return result, nil
}

func (api *MachinerAPI) SetObservedNetworkConfig(args params.SetMachineNetworkConfig) error {
	m, err := api.getMachineForSettingNetworkConfig(args.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	if m.IsContainer() {
		return nil
	}
	observedConfig := args.Config
	logger.Tracef("observed network config of machine %q: %+v", m.Id(), observedConfig)
	if len(observedConfig) == 0 {
		logger.Infof("not updating machine network config: no observed network config found")
		return nil
	}

	providerConfig, err := api.getOneMachineProviderNetworkConfig(m)
	if errors.IsNotProvisioned(err) {
		logger.Infof("not updating provider network config: %v", err)
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	if len(providerConfig) == 0 {
		logger.Infof("not updating machine network config: no provider network config found")
		return nil
	}

	mergedConfig := networkingcommon.MergeProviderAndObservedNetworkConfigs(providerConfig, observedConfig)
	logger.Tracef("merged observed and provider network config: %+v", mergedConfig)

	return api.setOneMachineNetworkConfig(m, mergedConfig)
}

func (api *MachinerAPI) getMachineForSettingNetworkConfig(machineTag string) (*state.Machine, error) {
	canModify, err := api.getCanModify()
	if err != nil {
		return nil, errors.Trace(err)
	}

	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !canModify(tag) {
		return nil, errors.Trace(common.ErrPerm)
	}

	m, err := api.getMachine(tag)
	if errors.IsNotFound(err) {
		return nil, errors.Trace(common.ErrPerm)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if m.IsContainer() {
		logger.Warningf("not updating network config for container %q", m.Id())
	}

	return m, nil
}

func (api *MachinerAPI) setOneMachineNetworkConfig(m *state.Machine, networkConfig []params.NetworkConfig) error {
	devicesArgs, devicesAddrs := networkingcommon.NetworkConfigsToStateArgs(networkConfig)

	logger.Debugf("setting devices: %+v", devicesArgs)
	if err := m.SetParentLinkLayerDevicesBeforeTheirChildren(devicesArgs); err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("setting addresses: %+v", devicesAddrs)
	if err := m.SetDevicesAddressesIdempotently(devicesAddrs); err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("updated machine %q network config", m.Id())
	return nil
}

func (api *MachinerAPI) SetProviderNetworkConfig(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	for i, arg := range args.Entities {
		m, err := api.getMachineForSettingNetworkConfig(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		if m.IsContainer() {
			continue
		}

		providerConfig, err := api.getOneMachineProviderNetworkConfig(m)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		} else if len(providerConfig) == 0 {
			continue
		}

		logger.Tracef("provider network config for %q: %+v", m.Id(), providerConfig)

		if err := api.setOneMachineNetworkConfig(m, providerConfig); err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return result, nil
}

func (api *MachinerAPI) getOneMachineProviderNetworkConfig(m *state.Machine) ([]params.NetworkConfig, error) {
	instId, err := m.InstanceId()
	if err != nil {
		return nil, errors.Trace(err)
	}

	netEnviron, err := networkingcommon.NetworkingEnvironFromModelConfig(
		stateenvirons.EnvironConfigGetter{api.st},
	)
	if errors.IsNotSupported(err) {
		logger.Infof("not updating provider network config: %v", err)
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get provider network config")
	}

	interfaceInfos, err := netEnviron.NetworkInterfaces(instId)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get network interfaces of %q", instId)
	}
	if len(interfaceInfos) == 0 {
		logger.Infof("not updating provider network config: no interfaces returned")
		return nil, nil
	}

	providerConfig := networkingcommon.NetworkConfigFromInterfaceInfo(interfaceInfos)
	logger.Tracef("provider network config instance %q: %+v", instId, providerConfig)

	return providerConfig, nil
}
