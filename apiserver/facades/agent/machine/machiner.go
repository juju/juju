// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// MachinerAPI implements the API used by the machiner worker.
type MachinerAPI struct {
	*common.LifeGetter
	*common.StatusSetter
	*common.DeadEnsurer
	*common.AgentEntityWatcher
	*common.APIAddresser
	*networkingcommon.NetworkConfigAPI

	st           *state.State
	auth         facade.Authorizer
	getCanModify common.GetAuthFunc
	getCanRead   common.GetAuthFunc
}

// NewMachinerAPIForState creates a new instance of the Machiner API.
func NewMachinerAPIForState(ctrlSt, st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*MachinerAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	getCanAccess := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}

	netConfigAPI, err := networkingcommon.NewNetworkConfigAPI(st, getCanAccess)
	if err != nil {
		return nil, errors.Annotate(err, "instantiating network config API")
	}

	return &MachinerAPI{
		LifeGetter:         common.NewLifeGetter(st, getCanAccess),
		StatusSetter:       common.NewStatusSetter(st, getCanAccess),
		DeadEnsurer:        common.NewDeadEnsurer(st, nil, getCanAccess),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, getCanAccess),
		APIAddresser:       common.NewAPIAddresser(ctrlSt, resources),
		NetworkConfigAPI:   netConfigAPI,
		st:                 st,
		auth:               authorizer,
		getCanModify:       getCanAccess,
		getCanRead:         getCanAccess,
	}, nil
}

func (api *MachinerAPI) getMachine(tag string, authChecker common.AuthFunc) (*state.Machine, error) {
	mtag, err := names.ParseMachineTag(tag)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	} else if !authChecker(mtag) {
		return nil, apiservererrors.ErrPerm
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
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		addresses, err := params.ToProviderAddresses(arg.Addresses...).ToSpaceAddresses(api.st)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := m.SetMachineAddresses(addresses...); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
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
			result.Results[i].Error = apiservererrors.ServerError(err)
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
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := m.RecordAgentStartInformation(""); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// RecordAgentStartInformation syncs the machine model with information
// reported by a machine agent when it starts.
func (api *MachinerAPI) RecordAgentStartInformation(args params.RecordAgentStartInformationArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	canModify, err := api.getCanModify()
	if err != nil {
		return results, err
	}

	for i, arg := range args.Args {
		m, err := api.getMachine(arg.Tag, canModify)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := m.RecordAgentStartInformation(arg.Hostname); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}
