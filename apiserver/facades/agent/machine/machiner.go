// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The machiner package implements the API interface
// used by the machiner worker.
package machine

import (
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
)

// NetworkConfigAPI is an interface that represents an version independent
// NetworkConfigAPI.
type NetworkConfigAPI interface {
	// SetObservedNetworkConfig reads the network config for the machine
	// identified by the input args. This config is merged with the new network
	// config supplied in the same args and updated if it has changed.
	SetObservedNetworkConfig(params.SetMachineNetworkConfig) error

	// SetProviderNetworkConfig sets the provider supplied network configuration
	// contained in the input args against each machine supplied with said args.
	SetProviderNetworkConfig(params.Entities) (params.ErrorResults, error)
}

// NetworkConfigAPIFunc creates a new NetworkConfigAPI independent of a version.
type NetworkConfigAPIFunc = func(*state.State, common.GetAuthFunc) NetworkConfigAPI

var logger = loggo.GetLogger("juju.apiserver.machine")

// MachinerAPI implements the API used by the machiner worker.
type MachinerAPI struct {
	*common.LifeGetter
	*common.StatusSetter
	*common.DeadEnsurer
	*common.AgentEntityWatcher
	*common.APIAddresser
	NetworkConfigAPI

	st           *state.State
	auth         facade.Authorizer
	getCanModify common.GetAuthFunc
	getCanRead   common.GetAuthFunc
}

// NewMachinerAPI creates a new instance of the V2 Machiner API.
func NewMachinerAPI(st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	networkConfigAPIFunc NetworkConfigAPIFunc,
) (*MachinerAPI, error) {
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
		NetworkConfigAPI:   networkConfigAPIFunc(st, getCanModify),
		st:                 st,
		auth:               authorizer,
		getCanModify:       getCanModify,
		getCanRead:         getCanRead,
	}, nil
}

func (api *MachinerAPI) getMachine(tag string, authChecker common.AuthFunc) (*state.Machine, error) {
	mtag, err := names.ParseMachineTag(tag)
	if err != nil {
		return nil, common.ErrPerm
	} else if !authChecker(mtag) {
		return nil, common.ErrPerm
	}

	entity, err := api.st.FindEntity(mtag)
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
		m, err := api.getMachine(arg.Tag, canModify)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		addresses, err := params.ToProviderAddresses(arg.Addresses...).ToSpaceAddresses(api.st)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := m.SetMachineAddresses(addresses...); err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
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
		machine, err := api.getMachine(agent.Tag, canRead)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		machineJobs := machine.Jobs()
		jobs := make([]model.MachineJob, len(machineJobs))
		for i, job := range machineJobs {
			jobs[i] = job.ToParams()
		}
		result.Results[i].Jobs = jobs
	}
	return result, nil
}

// RecordAgentStartTime updates the agent start time field in the machine doc.
func (api *MachinerAPI) RecordAgentStartTime(args params.Entities) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canModify, err := api.getCanModify()
	if err != nil {
		return results, err
	}

	for i, entity := range args.Entities {
		m, err := api.getMachine(entity.Tag, canModify)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := m.RecordAgentStartTime(); err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}

// MachinerAPIV1 implements the V1 API used by the machiner worker. Compared to
// V2, it lacks the RecordAgentStartTime method.
type MachinerAPIV1 struct {
	*MachinerAPIV2
}

// MachinerAPIV2 implements the V1 API used by the machiner worker. Compared to
// V2, it backfills the missing origin in NetworkConfig.
type MachinerAPIV2 struct {
	*MachinerAPI
}

// NewMachinerAPIV1 creates a new instance of the V1 Machiner API.
func NewMachinerAPIV1(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*MachinerAPIV1, error) {
	api, err := NewMachinerAPI(st, resources, authorizer, func(st *state.State, fn common.GetAuthFunc) NetworkConfigAPI {
		return networkingcommon.NewNetworkConfigAPIV1(st, fn)
	})
	if err != nil {
		return nil, err
	}

	// TODO (stickupkid): I'm not a fan of this, but I don't see a way to
	// compose two (APIV1 and APIV2) together when we depend on an API
	// constructor that remains the same.
	return &MachinerAPIV1{&MachinerAPIV2{api}}, nil
}

// NewMachinerAPIV2 creates a new instance of the V2 Machiner API.
func NewMachinerAPIV2(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*MachinerAPIV2, error) {
	api, err := NewMachinerAPI(st, resources, authorizer, func(st *state.State, fn common.GetAuthFunc) NetworkConfigAPI {
		return networkingcommon.NewNetworkConfigAPIV2(st, fn)
	})
	if err != nil {
		return nil, err
	}

	return &MachinerAPIV2{api}, nil
}

// RecordAgentStartTime is not available in V1.
func (api *MachinerAPIV1) RecordAgentStartTime(_, _ struct{}) {}
