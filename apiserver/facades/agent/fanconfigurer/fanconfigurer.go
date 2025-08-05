// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"strconv"
	"strings"

	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
)

// FanConfigurerAPIV1 implements the V1 api.
type FanConfigurerAPIV1 struct {
	*FanConfigurerAPI
}

// FanConfigurerAPI implements the latest api.
type FanConfigurerAPI struct {
	model           ModelAccessor
	machineAccessor MachineAccessor
	resources       facade.Resources
}

// NewFanConfigurerAPIForModel returns a new fan configurer api.
func NewFanConfigurerAPIForModel(
	model ModelAccessor, machineAccessor MachineAccessor, resources facade.Resources, authorizer facade.Authorizer,
) (*FanConfigurerAPI, error) {
	// Only machine agents have access to the fanconfigurer service.
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &FanConfigurerAPI{
		model:           model,
		machineAccessor: machineAccessor,
		resources:       resources,
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
func (m *FanConfigurerAPIV1) FanConfig() (params.FanConfigResult, error) {
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

// FanConfig returns current FAN configuration for the specified machine.
func (m *FanConfigurerAPI) FanConfig(arg params.Entity) (params.FanConfigResult, error) {
	result := params.FanConfigResult{}
	mTag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		return result, err
	}
	machine, err := m.machineAccessor.Machine(mTag.Id())
	if err != nil {
		return result, err
	}
	// For Ubuntu 24.04 and later, fan is not supported.
	// So if the machine for which the fan config is requested
	// is such a machine, return empty config.
	base := machine.Base()
	if base.OS != "ubuntu" {
		return result, nil
	}
	parts := strings.Split(base.Channel, ".")
	if baseMajor, err := strconv.Atoi(parts[0]); err == nil {
		if baseMajor >= 24 {
			return result, nil
		}
	}

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
