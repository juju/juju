// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("Undertaker", 1, NewUndertakerAPI)
}

// UndertakerAPI implements the API used by the machine undertaker worker.
type UndertakerAPI struct {
	st        State
	resources *common.Resources
}

// NewUndertakerAPI creates a new instance of the undertaker API.
func NewUndertakerAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UndertakerAPI, error) {
	return newUndertakerAPI(&stateShim{st}, resources, authorizer)
}

func newUndertakerAPI(st State, resources *common.Resources, authorizer common.Authorizer) (*UndertakerAPI, error) {
	if !authorizer.AuthMachineAgent() || !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}
	return &UndertakerAPI{
		st:        st,
		resources: resources,
	}, nil
}

// EnvironInfo returns information on the environment needed by the undertaker worker.
func (u *UndertakerAPI) EnvironInfo() (params.UndertakerEnvironInfoResult, error) {
	result := params.UndertakerEnvironInfoResult{}
	env, err := u.st.Environment()

	if err != nil {
		return result, errors.Trace(err)
	}
	tod := env.TimeOfDeath()

	result.Result = params.UndertakerEnvironInfo{
		UUID:        env.UUID(),
		GlobalName:  env.Owner().String() + "/" + env.Name(),
		Name:        env.Name(),
		IsSystem:    u.st.IsStateServer(),
		Life:        params.Life(env.Life().String()),
		TimeOfDeath: &tod,
	}
	if tod.IsZero() {
		result.Result.TimeOfDeath = nil
	}

	return result, nil
}

// ProcessDyingEnviron checks if a dying environment has any machines or services.
// If there are none, the environment's life is changed from dying to dead.
func (u *UndertakerAPI) ProcessDyingEnviron() error {
	return u.st.ProcessDyingEnviron()
}

// RemoveEnviron removes any records of this environment from Juju.
func (u *UndertakerAPI) RemoveEnviron() error {
	err := u.st.RemoveAllEnvironDocs()
	if err != nil {
		// TODO(waigani) Return a human friendly error for now. The proper fix
		// is to run a buildTxn within state.RemoveAllEnvironDocs, so we
		// can return better errors than "transaction aborted".
		return errors.New("an error occurred, unable to remove environment")
	}
	return nil
}

func (u *UndertakerAPI) environResourceWatcher() params.NotifyWatchResult {
	var nothing params.NotifyWatchResult
	machines, err := u.st.AllMachines()
	if err != nil {
		nothing.Error = common.ServerError(err)
		return nothing
	}
	services, err := u.st.AllServices()
	if err != nil {
		nothing.Error = common.ServerError(err)
		return nothing
	}
	var watchers []state.NotifyWatcher
	for _, machine := range machines {
		watchers = append(watchers, machine.Watch())
	}
	for _, service := range services {
		watchers = append(watchers, service.Watch())
	}

	watch := common.NewMultiNotifyWatcher(watchers...)

	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: u.resources.Register(watch),
		}
	}
	nothing.Error = common.ServerError(watcher.EnsureErr(watch))
	return nothing
}

// WatchEnvironResources creates watchers for changes to the lifecycle of an
// environment's machines and services.
func (u *UndertakerAPI) WatchEnvironResources() params.NotifyWatchResults {
	return params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			u.environResourceWatcher(),
		},
	}
}
