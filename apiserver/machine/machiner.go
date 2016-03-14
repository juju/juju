// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The machiner package implements the API interface
// used by the machiner worker.
package machine

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
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
	auth         common.Authorizer
	getCanModify common.GetAuthFunc
	getCanRead   common.GetAuthFunc
}

// NewMachinerAPI creates a new instance of the Machiner API.
func NewMachinerAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*MachinerAPI, error) {
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
				addresses := params.NetworkAddresses(arg.Addresses)
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
	canModify, err := api.getCanModify()
	if err != nil {
		return errors.Trace(err)
	}
	tag, err := names.ParseMachineTag(args.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canModify(tag) {
		return errors.Trace(common.ErrPerm)
	}
	m, err := api.getMachine(tag)
	if errors.IsNotFound(err) {
		return errors.Trace(common.ErrPerm)
	} else if err != nil {
		return errors.Trace(err)
	}
	observedConfig := args.Config
	logger.Tracef("observed network config of machine %q: %+v", m.Id(), observedConfig)

	// Get the provider network config to combine with the observed config.
	instId, err := m.InstanceId()
	if err != nil {
		return errors.Trace(err)
	}
	netEnviron, err := networkingcommon.NetworkingEnvironFromModelConfig(api.st)
	if errors.IsNotSupported(err) {
		logger.Debugf("not merging observed and provider network config: %v", err)
	} else if err != nil {
		return errors.Annotate(err, "cannot get provider network config")
	}
	interfaceInfos, err := netEnviron.NetworkInterfaces(instId)
	if err != nil {
		return errors.Annotatef(err, "cannot get network interfaces of %q", instId)
	}
	if len(interfaceInfos) == 0 {
		logger.Warningf("instance %q has no network interfaces; using observed only", instId)
	}
	providerConfig := networkingcommon.NetworkConfigFromInterfaceInfo(interfaceInfos)
	logger.Tracef("provider network config instance %q: %+v", instId, observedConfig)

	mergedConfig := networkingcommon.MergeProviderAndObservedNetworkConfigs(providerConfig, observedConfig)
	logger.Tracef("merged network config: %+v", instId, mergedConfig)

	// devicesArgs, devicesAddrs := networkingcommon.NetworkConfigsToStateArgs(mergedConfig)

	// // Becausee we can't add parent and children devices in one call, we need to
	// // split the devicesArgs into multiple calls. As the config and args are
	// // already sorted, we can make a call once the parent changes.
	// var pendingArgs []state.LinkLayerDeviceArgs
	// lastParent := ""
	// for _, deviceArgs := range devicesArgs {
	// 	if lastParent != deviceArgs.ParentName && len(pendingArgs) > 0 {
	// 		logger.Debugf("adding devices: %+v", pendingArgs)
	// 		if err := m.AddLinkLayerDevices(pendingArgs...); err != nil {
	// 			return errors.Trace(err)
	// 		}
	// 		pendingArgs = []state.LinkLayerDeviceArgs{}
	// 	}

	// 	pendingArgs = append(pendingArgs, deviceArgs)
	// 	lastParent = deviceArgs.ParentName
	// }
	// if len(pendingArgs) > 0 {
	// 	logger.Debugf("adding devices: %+v", pendingArgs)
	// 	if err := m.AddLinkLayerDevices(pendingArgs...); err != nil {
	// 		return errors.Trace(err)
	// 	}
	// }

	// logger.Debugf("setting addresses: %+v", devicesAddrs)
	// if err := m.SetDevicesAddresses(devicesAddrs...); err != nil {
	// 	return errors.Trace(err)
	// }

	logger.Debugf("updated machine %q network config", m.Id())
	return nil
}
