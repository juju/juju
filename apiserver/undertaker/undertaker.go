// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("Undertaker", 1, NewUndertakerAPI)
}

// UndertakerAPI implements the API used by the model undertaker worker.
type UndertakerAPI struct {
	st        State
	resources facade.Resources
	*common.StatusSetter
}

// NewUndertakerAPI creates a new instance of the undertaker API.
func NewUndertakerAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*UndertakerAPI, error) {
	return newUndertakerAPI(&stateShim{st}, resources, authorizer)
}

func newUndertakerAPI(st State, resources facade.Resources, authorizer facade.Authorizer) (*UndertakerAPI, error) {
	if !authorizer.AuthMachineAgent() || !authorizer.AuthModelManager() {
		return nil, common.ErrPerm
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	getCanModifyModel := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if st.IsController() {
				return true
			}
			// Only the agent's model can be modified.
			modelTag, ok := tag.(names.ModelTag)
			if !ok {
				return false
			}
			return modelTag.Id() == model.UUID()
		}, nil
	}
	return &UndertakerAPI{
		st:           st,
		resources:    resources,
		StatusSetter: common.NewStatusSetter(st, getCanModifyModel),
	}, nil
}

// ModelInfo returns information on the model needed by the undertaker worker.
func (u *UndertakerAPI) ModelInfo() (params.UndertakerModelInfoResult, error) {
	result := params.UndertakerModelInfoResult{}
	env, err := u.st.Model()

	if err != nil {
		return result, errors.Trace(err)
	}

	result.Result = params.UndertakerModelInfo{
		UUID:       env.UUID(),
		GlobalName: env.Owner().String() + "/" + env.Name(),
		Name:       env.Name(),
		IsSystem:   u.st.IsController(),
		Life:       params.Life(env.Life().String()),
	}

	return result, nil
}

// ProcessDyingModel checks if a dying environment has any machines or services.
// If there are none, the environment's life is changed from dying to dead.
func (u *UndertakerAPI) ProcessDyingModel() error {
	return u.st.ProcessDyingModel()
}

// RemoveModel removes any records of this model from Juju.
func (u *UndertakerAPI) RemoveModel() error {
	err := u.st.RemoveAllModelDocs()
	if err != nil {
		// TODO(waigani) Return a human friendly error for now. The proper fix
		// is to run a buildTxn within state.RemoveAllModelDocs, so we
		// can return better errors than "transaction aborted".
		return errors.New("an error occurred, unable to remove model")
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
	services, err := u.st.AllApplications()
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

// WatchModelResources creates watchers for changes to the lifecycle of an
// model's machines and services.
func (u *UndertakerAPI) WatchModelResources() params.NotifyWatchResults {
	return params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			u.environResourceWatcher(),
		},
	}
}

// ModelConfig returns the model's configuration.
func (u *UndertakerAPI) ModelConfig() (params.ModelConfigResult, error) {
	result := params.ModelConfigResult{}

	config, err := u.st.ModelConfig()
	if err != nil {
		return result, err
	}
	allAttrs := config.AllAttrs()
	result.Config = allAttrs
	return result, nil
}
