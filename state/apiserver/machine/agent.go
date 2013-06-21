// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The machine package implements the API interfaces
// used by the machine agent.
package machine

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

type AgentAPI struct {
	st   *state.State
	auth common.Authorizer
}

// NewAgentAPI returns an object implementing the machine agent API
// with the given authorizer representing the currently logged in client.
func NewAgentAPI(st *state.State, auth common.Authorizer) (*AgentAPI, error) {
	if !auth.IsLoggedIn() {
		return nil, common.ErrNotLoggedIn
	}
	if !auth.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	return &AgentAPI{
		st:   st,
		auth: auth,
	}, nil
}

func (api *AgentAPI) GetMachines(args params.Machines) (params.MachineAgentGetMachinesResults, error) {
	results := params.MachineAgentGetMachinesResults{
		Machines: make([]params.MachineAgentGetMachinesResult, len(args.Ids)),
	}
	for i, id := range args.Ids {
		result, err := api.getMachine(id)
		result.Error = common.ServerError(err)
		results.Machines[i] = result
	}
	return results, nil
}

func (api *AgentAPI) getMachine(id string) (result params.MachineAgentGetMachinesResult, err error) {
	// Allow only for the owner agent.
	// Note: having a bulk API call for this is utter madness, given that
	// this check means we can only ever return a single object.
	if !api.auth.AuthOwner(state.MachineTag(id)) {
		err = common.ErrPerm
		return
	}
	machine, err := api.st.Machine(id)
	if err != nil {
		return
	}
	result.Life = params.Life(machine.Life().String())
	result.Jobs = stateJobsToAPIParamsJobs(machine.Jobs())
	return
}

func stateJobsToAPIParamsJobs(jobs []state.MachineJob) []params.MachineJob {
	pjobs := make([]params.MachineJob, len(jobs))
	for i, job := range jobs {
		pjobs[i] = params.MachineJob(job.String())
	}
	return pjobs
}
