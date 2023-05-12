// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state/watcher"
)

// For testing.
var (
	GetProvider = provider.Provider
)

// UndertakerAPI implements the API used by the model undertaker worker.
type UndertakerAPI struct {
	st        State
	resources facade.Resources
	*common.StatusSetter

	secretBackendConfigGetter commonsecrets.BackendAdminConfigGetter
}

func newUndertakerAPI(st State, resources facade.Resources, authorizer facade.Authorizer, secretBackendConfigGetter commonsecrets.BackendAdminConfigGetter) (*UndertakerAPI, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
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
		st:                        st,
		resources:                 resources,
		secretBackendConfigGetter: secretBackendConfigGetter,
		StatusSetter:              common.NewStatusSetter(st, getCanModifyModel),
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
		DestroyTimeout: model.DestroyTimeout(),
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
	secretBackendCfg, err := u.secretBackendConfigGetter()
	if err != nil {
		return errors.Annotate(err, "getting secrets backends config")
	}
	for _, cfg := range secretBackendCfg.Configs {
		if err := u.removeModelSecrets(&cfg); err != nil {
			return errors.Annotatef(err, "cleaning model from inactive secrets provider %q", cfg.BackendType)
		}
	}
	return u.st.RemoveDyingModel()
}

func (u *UndertakerAPI) removeModelSecrets(cfg *provider.ModelBackendConfig) error {
	p, err := GetProvider(cfg.BackendType)
	if err != nil {
		return errors.Trace(err)
	}
	return p.CleanupModel(cfg)
}

func (u *UndertakerAPI) modelEntitiesWatcher() params.NotifyWatchResult {
	var nothing params.NotifyWatchResult
	watch := u.st.WatchModelEntityReferences(u.st.ModelUUID())
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: u.resources.Register(watch),
		}
	}
	nothing.Error = apiservererrors.ServerError(watcher.EnsureErr(watch))
	return nothing
}

// WatchModelResources creates watchers for changes to the lifecycle of an
// model's machines and applications and storage.
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
