// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spacenamer

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
)

//go:generate mockgen -package mocks -destination mocks/spacenamer_mock.go github.com/juju/juju/apiserver/facades/agent/spacenamer SpaceNamerState,Space,Model,Config
//go:generate mockgen -package mocks -destination mocks/modelcache_mock.go github.com/juju/juju/apiserver/facades/agent/spacenamer ModelCache
//go:generate mockgen -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/core/cache NotifyWatcher

type SpaceNamerAPIV1 struct {
	*SpaceNamerAPI
}

// NewFacade is used for API registration.
func NewFacade(ctx facade.Context) (*SpaceNamerAPI, error) {
	st := &spaceNamerStateShim{State: ctx.State()}
	model, err := ctx.Controller().Model(st.ModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelCache := model
	return NewSpaceNamerAPI(st, modelCache, ctx.Resources(), ctx.Auth())
}

// NewSpaceNamerAPI creates a new API server endpoint for setting
// the default space name for this model.
func NewSpaceNamerAPI(st SpaceNamerState,
	model ModelCache,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*SpaceNamerAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, common.ErrPerm
	}

	return &SpaceNamerAPI{
		st:         st,
		model:      model,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// WatchDefaultSpaceConfig starts a watcher to track changes to the DefaultSpace config.
func (api *SpaceNamerAPI) WatchDefaultSpaceConfig() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	if !api.authorizer.AuthController() {
		return result, common.ErrPerm
	}

	watch := api.model.WatchConfig(config.DefaultSpace)
	if _, ok := <-watch.Changes(); ok {
		// Consume the initial event. Technically, API calls to Watch
		// 'transmit' the initial event in the Watch response. But
		// NotifyWatchers have no state to transmit.
		result.NotifyWatcherId = api.resources.Register(watch)
	} else {
		result.Error = common.ServerError(errors.Errorf("programming error: channel should not be closed"))
	}

	return result, nil
}

// SetDefaultSpaceName sets the name of the model's default space.
func (api *SpaceNamerAPI) SetDefaultSpaceName() (params.ErrorResult, error) {
	result := params.ErrorResult{}
	if !api.authorizer.AuthController() {
		return result, common.ErrPerm
	}

	result.Error = common.ServerError(api.setDefaultSpaceName())
	return result, nil
}

func (api *SpaceNamerAPI) setDefaultSpaceName() error {
	m, err := api.st.Model()
	if err != nil {
		return err
	}

	cfg, err := m.Config()
	if err != nil {
		return err
	}

	sp, err := api.st.Space(network.DefaultSpaceId)
	if err != nil {
		return err
	}

	newName := cfg.DefaultSpace()
	if sp.Name() == newName {
		return nil
	}

	// TODO hml 2019-10-25
	// If this call fails due to name already in use or any other
	// reason, leaving the model-config value as is could cause issues
	// in other juju code believe the value is valid.
	// Unfortunately there is limited validation available for
	// config.Config.
	return sp.SetName(newName)
}
