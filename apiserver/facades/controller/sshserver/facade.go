// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// State provides required state for the Facade.
type State interface {
	WatchControllerConfig() state.NotifyWatcher
	SSHServerHostKey() (string, error)
}

// Facade allows model config manager clients to watch controller config changes and fetch controller config.
type Facade struct {
	resources facade.Resources

	ctrlState           State
	controllerConfigAPI *common.ControllerConfigAPI
}

func NewFacade(ctx facade.Context, systemState *state.State) *Facade {
	return &Facade{
		resources:           ctx.Resources(),
		controllerConfigAPI: common.NewStateControllerConfig(systemState),
		ctrlState:           systemState,
	}
}

func (f *Facade) ControllerConfig() (params.ControllerConfigResult, error) {
	return f.controllerConfigAPI.ControllerConfig()
}

func (f *Facade) WatchControllerConfig() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	w := f.ctrlState.WatchControllerConfig()
	if _, ok := <-w.Changes(); ok {
		result.NotifyWatcherId = f.resources.Register(w)
	} else {
		return result, watcher.EnsureErr(w)
	}
	return result, nil
}

func (f *Facade) SSHServerHostKey() (params.SSHServerHostPrivateKeyResult, error) {
	result := params.SSHServerHostPrivateKeyResult{}
	key, err := f.ctrlState.SSHServerHostKey()
	if err != nil {
		return result, err
	}
	result.HostKey = key
	return result, nil
}
