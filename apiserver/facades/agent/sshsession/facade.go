// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// Backend provides required state for the Facade.
type Backend interface {
	GetSSHConnRequest(docID string) (state.SSHConnRequest, error)
	WatchSSHConnRequest(machineId string) state.StringsWatcher
}

// Facade allows model config manager clients to watch controller config changes and fetch controller config.
type Facade struct {
	resources facade.Resources

	backend Backend
}

// newFacade creates the facade for the SSHSession.
func newFacade(ctx facade.Context, backend Backend) *Facade {
	return &Facade{
		resources: ctx.Resources(),
		backend:   backend,
	}
}

// GetSSHConnRequest returns a ssh connection request by its document ID.
func (f *Facade) GetSSHConnRequest(arg params.SSHConnRequestGetArg) (params.SSHConnRequestResult, error) {
	result := params.SSHConnRequestResult{}
	connReq, err := f.backend.GetSSHConnRequest(arg.RequestId)
	if err != nil {
		return result, err
	}
	result.SSHConnRequest = params.SSHConnRequest(connReq)
	return result, nil
}

// WatchSSHConnRequest creates a watcher and returns its ID for watching changes.
func (f *Facade) WatchSSHConnRequest(arg params.SSHConnRequestWatchArg) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	w := f.backend.WatchSSHConnRequest(arg.MachineId)
	if changes, ok := <-w.Changes(); ok {
		result.StringsWatcherId = f.resources.Register(w)
		result.Changes = changes
	} else {
		return result, watcher.EnsureErr(w)
	}
	return result, nil
}
