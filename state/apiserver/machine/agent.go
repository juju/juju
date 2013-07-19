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
	*common.PasswordChanger

	st        *state.State
	resources *common.Resources
	auth      common.Authorizer
}

// NewAgentAPI returns an object implementing the machine agent API
// with the given authorizer representing the currently logged in client.
func NewAgentAPI(st *state.State, resources *common.Resources, auth common.Authorizer) (*AgentAPI, error) {
	if !auth.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	getCanChange := func() (common.AuthFunc, error) {
		// TODO(go1.1): method expression
		return func(tag string) bool {
			return auth.AuthOwner(tag)
		}, nil
	}
	// The machine agent has connected, so start a pinger to announce
	// it's now alive.
	machine, err := st.Machine(state.MachineIdFromTag(auth.GetAuthTag()))
	if err != nil {
		return nil, err
	}
	pinger, err := machine.SetAgentAlive()
	if err != nil {
		return nil, err
	}
	resources.Register(pinger)
	return &AgentAPI{
		PasswordChanger: common.NewPasswordChanger(st, getCanChange),
		st:              st,
		resources:       resources,
		auth:            auth,
	}, nil
}

func (api *AgentAPI) GetMachines(args params.Entities) params.MachineAgentGetMachinesResults {
	results := params.MachineAgentGetMachinesResults{
		Machines: make([]params.MachineAgentGetMachinesResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		result, err := api.getMachine(entity.Tag)
		result.Error = common.ServerError(err)
		results.Machines[i] = result
	}
	return results
}

func (api *AgentAPI) getMachine(tag string) (result params.MachineAgentGetMachinesResult, err error) {
	// Allow only for the owner agent.
	// Note: having a bulk API call for this is utter madness, given that
	// this check means we can only ever return a single object.
	if !api.auth.AuthOwner(tag) {
		err = common.ErrPerm
		return
	}
	machine, err := api.st.Machine(state.MachineIdFromTag(tag))
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
