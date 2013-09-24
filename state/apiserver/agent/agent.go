// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The machine package implements the API interfaces
// used by the machine agent.
package agent

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

// API implements the API provided to an agent.
type API struct {
	*common.PasswordChanger

	st   *state.State
	auth common.Authorizer
}

// NewAPI returns an object implementing an agent API
// with the given authorizer representing the currently logged in client.
func NewAPI(st *state.State, auth common.Authorizer) (*API, error) {
	// Agents are defined to be any user that's not a client user.
	if !auth.AuthMachineAgent() && !auth.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	getCanChange := func() (common.AuthFunc, error) {
		return auth.AuthOwner, nil
	}
	return &API{
		PasswordChanger: common.NewPasswordChanger(st, getCanChange),
		st:              st,
		auth:            auth,
	}, nil
}

func (api *API) GetEntities(args params.Entities) params.AgentGetEntitiesResults {
	results := params.AgentGetEntitiesResults{
		Entities: make([]params.AgentGetEntitiesResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		result, err := api.getEntity(entity.Tag)
		result.Error = common.ServerError(err)
		results.Entities[i] = result
	}
	return results
}

func (api *API) getEntity(tag string) (result params.AgentGetEntitiesResult, err error) {
	// Allow only for the owner agent.
	// Note: having a bulk API call for this is utter madness, given that
	// this check means we can only ever return a single object.
	if !api.auth.AuthOwner(tag) {
		err = common.ErrPerm
		return
	}
	entity0, err := api.st.FindEntity(tag)
	if err != nil {
		return
	}
	entity, ok := entity0.(state.Lifer)
	if !ok {
		err = common.NotSupportedError(tag, "life cycles")
		return
	}
	result.Life = params.Life(entity.Life().String())
	if machine, ok := entity.(*state.Machine); ok {
		result.Jobs = stateJobsToAPIParamsJobs(machine.Jobs())
		result.ContainerType = machine.ContainerType()
	}
	return
}

func stateJobsToAPIParamsJobs(jobs []state.MachineJob) []params.MachineJob {
	pjobs := make([]params.MachineJob, len(jobs))
	for i, job := range jobs {
		pjobs[i] = params.MachineJob(job.String())
	}
	return pjobs
}
