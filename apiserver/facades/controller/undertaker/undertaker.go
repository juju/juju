// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// UndertakerAPI implements the API used by the model undertaker worker.
type UndertakerAPI struct {
	st        State
	resources facade.Resources
	*common.StatusSetter
}

// NewUndertakerAPI creates a new instance of the undertaker API.
func NewUndertakerAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*UndertakerAPI, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return newUndertakerAPI(&stateShim{st, m}, resources, authorizer)
}

func newUndertakerAPI(st State, resources facade.Resources, authorizer facade.Authorizer) (*UndertakerAPI, error) {
	if !authorizer.AuthController() {
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
	model, err := u.st.Model()

	if err != nil {
		return result, errors.Trace(err)
	}

	result.Result = params.UndertakerModelInfo{
		UUID:           model.UUID(),
		GlobalName:     model.Owner().String() + "/" + model.Name(),
		Name:           model.Name(),
		IsSystem:       u.st.IsController(),
		Life:           life.Value(model.Life().String()),
		ForceDestroyed: model.ForceDestroyed(),
	}

	return result, nil
}

// ProcessDyingModel checks if a dying model has any machines or applications.
// If there are none, the model's life is changed from dying to dead.
func (u *UndertakerAPI) ProcessDyingModel() error {
	return u.st.ProcessDyingModel()
}

// RemoveModel removes any records of this model from Juju.
func (u *UndertakerAPI) RemoveModel() error {
	return u.st.RemoveDyingModel()
}

func (u *UndertakerAPI) modelEntitiesWatcher() params.NotifyWatchResult {
	var nothing params.NotifyWatchResult
	watch := u.st.WatchModelEntityReferences(u.st.ModelUUID())
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
			u.modelEntitiesWatcher(),
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
