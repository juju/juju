// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package agent implements the API interfaces
// used by the machine agent.

package agent

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("Agent", 2, NewAgentAPIV2)
}

// AgentAPIV2 implements the version 2 of the API provided to an agent.
type AgentAPIV2 struct {
	*common.PasswordChanger
	*common.RebootFlagClearer
	*common.ModelWatcher
	*common.ControllerConfigAPI
	cloudspec.CloudSpecAPI

	st        *state.State
	auth      facade.Authorizer
	resources facade.Resources
}

// NewAgentAPIV2 returns an object implementing version 2 of the Agent API
// with the given authorizer representing the currently logged in client.
func NewAgentAPIV2(st *state.State, resources facade.Resources, auth facade.Authorizer) (*AgentAPIV2, error) {
	// Agents are defined to be any user that's not a client user.
	if !auth.AuthMachineAgent() && !auth.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	getCanChange := func() (common.AuthFunc, error) {
		return auth.AuthOwner, nil
	}
	environConfigGetter := stateenvirons.EnvironConfigGetter{st}
	return &AgentAPIV2{
		PasswordChanger:     common.NewPasswordChanger(st, getCanChange),
		RebootFlagClearer:   common.NewRebootFlagClearer(st, getCanChange),
		ModelWatcher:        common.NewModelWatcher(st, resources, auth),
		ControllerConfigAPI: common.NewControllerConfig(st),
		CloudSpecAPI:        cloudspec.NewCloudSpec(environConfigGetter.CloudSpec, common.AuthFuncForTag(st.ModelTag())),
		st:                  st,
		auth:                auth,
		resources:           resources,
	}, nil
}

func (api *AgentAPIV2) GetEntities(args params.Entities) params.AgentGetEntitiesResults {
	results := params.AgentGetEntitiesResults{
		Entities: make([]params.AgentGetEntitiesResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Entities[i].Error = common.ServerError(err)
			continue
		}
		result, err := api.getEntity(tag)
		result.Error = common.ServerError(err)
		results.Entities[i] = result
	}
	return results
}

func (api *AgentAPIV2) getEntity(tag names.Tag) (result params.AgentGetEntitiesResult, err error) {
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

func (api *AgentAPIV2) StateServingInfo() (result params.StateServingInfo, err error) {
	if !api.auth.AuthModelManager() {
		err = common.ErrPerm
		return
	}
	info, err := api.st.StateServingInfo()
	if err != nil {
		return params.StateServingInfo{}, errors.Trace(err)
	}
	result = params.StateServingInfo{
		APIPort:        info.APIPort,
		StatePort:      info.StatePort,
		Cert:           info.Cert,
		PrivateKey:     info.PrivateKey,
		CAPrivateKey:   info.CAPrivateKey,
		SharedSecret:   info.SharedSecret,
		SystemIdentity: info.SystemIdentity,
	}

	return result, nil
}

// MongoIsMaster is called by the IsMaster API call
// instead of mongo.IsMaster. It exists so it can
// be overridden by tests.
var MongoIsMaster = mongo.IsMaster

func (api *AgentAPIV2) IsMaster() (params.IsMasterResult, error) {
	if !api.auth.AuthModelManager() {
		return params.IsMasterResult{}, common.ErrPerm
	}

	switch tag := api.auth.GetAuthTag().(type) {
	case names.MachineTag:
		machine, err := api.st.Machine(tag.Id())
		if err != nil {
			return params.IsMasterResult{}, common.ErrPerm
		}

		session := api.st.MongoSession()
		isMaster, err := MongoIsMaster(session, machine)
		return params.IsMasterResult{Master: isMaster}, err
	default:
		return params.IsMasterResult{}, errors.Errorf("authenticated entity is not a Machine")
	}
}

func stateJobsToAPIParamsJobs(jobs []state.MachineJob) []multiwatcher.MachineJob {
	pjobs := make([]multiwatcher.MachineJob, len(jobs))
	for i, job := range jobs {
		pjobs[i] = multiwatcher.MachineJob(job.String())
	}
	return pjobs
}

// WatchCredentials watches for changes to the specified credentials.
func (api *AgentAPIV2) WatchCredentials(args params.Entities) (params.NotifyWatchResults, error) {
	if !api.auth.AuthModelManager() {
		return params.NotifyWatchResults{}, common.ErrPerm
	}

	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		credentialTag, err := names.ParseCloudCredentialTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		watch := api.st.WatchCredential(credentialTag)
		// Consume the initial event. Technically, API calls to Watch
		// 'transmit' the initial event in the Watch response. But
		// NotifyWatchers have no state to transmit.
		if _, ok := <-watch.Changes(); ok {
			results.Results[i].NotifyWatcherId = api.resources.Register(watch)
		} else {
			err = watcher.EnsureErr(watch)
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}
