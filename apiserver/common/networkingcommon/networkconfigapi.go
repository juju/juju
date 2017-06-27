// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The networkconfigapi package implements the network config parts
// common to machiner and provisioner interface

package networkingcommon

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

type NetworkConfigAPI struct {
	st           *state.State
	getCanModify common.GetAuthFunc
}

func NewNetworkConfigAPI(st *state.State, getCanModify common.GetAuthFunc) *NetworkConfigAPI {
	return &NetworkConfigAPI{
		st:           st,
		getCanModify: getCanModify,
	}
}

func (api *NetworkConfigAPI) getMachine(tag names.MachineTag) (*state.Machine, error) {
	entity, err := api.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	return entity.(*state.Machine), nil
}

func (api *NetworkConfigAPI) getOneMachineProviderNetworkConfig(m *state.Machine) ([]params.NetworkConfig, error) {
	manual, err := m.IsManual()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if manual {
		logger.Infof("provider network config not supported on manually provisioned machines")
		return nil, nil
	}

	instId, err := m.InstanceId()
	if err != nil {
		return nil, errors.Trace(err)
	}

	netEnviron, err := NetworkingEnvironFromModelConfig(
		stateenvirons.EnvironConfigGetter{api.st},
	)
	if errors.IsNotSupported(err) {
		logger.Infof("provider network config not supported: %v", err)
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get provider network config")
	}

	interfaceInfos, err := netEnviron.NetworkInterfaces(instId)
	if errors.IsNotSupported(err) {
		// It's possible to have a networking environ, but not support
		// NetworkInterfaces().  In leiu of adding SupportsNetworkInterfaces():
		logger.Infof("provider network interfaces not supported: %v", err)
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot get network interfaces of %q", instId)
	}
	if len(interfaceInfos) == 0 {
		logger.Infof("not updating provider network config: no interfaces returned")
		return nil, nil
	}

	providerConfig := NetworkConfigFromInterfaceInfo(interfaceInfos)
	logger.Tracef("provider network config instance %q: %+v", instId, providerConfig)

	return providerConfig, nil
}

func (api *NetworkConfigAPI) setOneMachineNetworkConfig(m *state.Machine, networkConfig []params.NetworkConfig) error {
	devicesArgs, devicesAddrs := NetworkConfigsToStateArgs(networkConfig)

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

func (api *NetworkConfigAPI) getMachineForSettingNetworkConfig(machineTag string) (*state.Machine, error) {
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
		logger.Debugf("not updating network config for container %q", m.Id())
	}

	return m, nil
}

func (api *NetworkConfigAPI) SetObservedNetworkConfig(args params.SetMachineNetworkConfig) error {
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
		logger.Infof("not updating machine %q network config: no observed network config found", m.Id())
		return nil
	}

	providerConfig, err := api.getOneMachineProviderNetworkConfig(m)
	if errors.IsNotProvisioned(err) {
		logger.Infof("not updating machine %q network config: %v", m.Id(), err)
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	finalConfig := observedConfig
	if len(providerConfig) != 0 {
		finalConfig = MergeProviderAndObservedNetworkConfigs(providerConfig, observedConfig)
		logger.Tracef("merged observed and provider network config for machine %q: %+v", m.Id(), finalConfig)
	}

	return api.setOneMachineNetworkConfig(m, finalConfig)
}

func (api *NetworkConfigAPI) SetProviderNetworkConfig(args params.Entities) (params.ErrorResults, error) {
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
