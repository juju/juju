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

	devicesArgs, devicesAddrs := networkingcommon.NetworkConfigToStateArgs(args.Config)
	logger.Debugf("all observed devices on machine %q: %+v", m.Id(), devicesArgs)
	logger.Debugf("about to set devices addresses: %+v", devicesAddrs)

	// Filter all but bridge devices as we only need those for multi-NIC
	// containers.
	var bridgeDevicesArgs []state.LinkLayerDeviceArgs
	for _, deviceArgs := range devicesArgs {
		if deviceArgs.Type != state.BridgeDevice {
			// FIXME: Loopback shouldn't be skipped.
			logger.Debugf("skipping non-bridge device %q", deviceArgs.Name)
			continue
		}
		logger.Debugf("will add observed device: %+v", deviceArgs)
		bridgeDevicesArgs = append(bridgeDevicesArgs, deviceArgs)
	}
	if err := m.AddLinkLayerDevices(bridgeDevicesArgs...); errors.IsAlreadyExists(err) {
		// FIXME: Trying to add the same device more than once should be handled
		// better - as an update rather than insert.
		logger.Warningf("ignoring non-fatal error: %v", err)
	} else if err != nil {
		return errors.Trace(err)
	}

	if err := m.SetDevicesAddresses(devicesAddrs...); errors.IsNotFound(err) {
		// FIXME: device not found here should be handled better.
		logger.Warningf("ignoring non-fatal error: %v", err)
	} else if err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("updated machine %q network config", m.Id())
	return nil
}
