// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package fanconfigurer

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// FanConfigurer defines the methods on fanconfigurer API endpoint.
type FanConfigurer interface {
	WatchForFanConfigChanges() (params.NotifyWatchResult, error)
	FanConfig() (params.FanConfigResult, error)
}

type FanConfigurerAPI struct {
	model     state.ModelAccessor
	resources facade.Resources
}

var _ FanConfigurer = (*FanConfigurerAPI)(nil)

// NewFanConfigurerAPI creates a new FanConfigurer API endpoint on server-side.
func NewFanConfigurerAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*FanConfigurerAPI, error) {
	model, err := st.Model()
	if err != nil {
		return nil, err
	}
	return NewFanConfigurerAPIForModel(model, resources, authorizer)
}

func NewFanConfigurerAPIForModel(model state.ModelAccessor, resources facade.Resources, authorizer facade.Authorizer) (*FanConfigurerAPI, error) {
	// Only machine agents have access to the fanconfigurer service.
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}

	return &FanConfigurerAPI{
		model:     model,
		resources: resources,
	}, nil
}

// WatchForFanConfigChanges returns a NotifyWatcher that observes
// changes to the FAN configuration.
// so we use the regular error return.
// TODO(wpk) 2017-09-21 We should use Model directly, and watch only for FanConfig changes.
func (m *FanConfigurerAPI) WatchForFanConfigChanges() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	watch := m.model.WatchForModelConfigChanges()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = m.resources.Register(watch)
	} else {
		return result, watcher.EnsureErr(watch)
	}
	return result, nil
}

// FanConfig returns current FAN configuration.
func (m *FanConfigurerAPI) FanConfig() (params.FanConfigResult, error) {
	result := params.FanConfigResult{}
	config, err := m.model.ModelConfig()
	if err != nil {
		return result, err
	}
	fanConfig, err := config.FanConfig()
	if err != nil {
		return result, err
	}
	return networkingcommon.FanConfigToFanConfigResult(fanConfig), nil
}
