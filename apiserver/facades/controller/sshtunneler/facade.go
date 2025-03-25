// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// Backend provides required state for the Facade.
type Backend interface {
	InsertSSHConnRequest(arg state.SSHConnRequestArg) error
	RemoveSSHConnRequest(arg state.SSHConnRequestRemoveArg) error
}

// Facade is the interface exposing the SSHTunneler methods.
type Facade struct {
	backend Backend
}

// newFacade creates the facade for the SSHTunneler.
func newFacade(ctx facade.Context, backend Backend) *Facade {
	return &Facade{
		backend: backend,
	}
}

// InsertSSHConnRequest inserts a new ssh connection request in the state.
func (f *Facade) InsertSSHConnRequest(arg params.SSHConnRequestArg) (params.ErrorResult, error) {
	err := f.backend.InsertSSHConnRequest(state.SSHConnRequestArg(arg))
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

// RemoveSSHConnRequest removes a ssh connection request from the state.
func (f *Facade) RemoveSSHConnRequest(arg params.SSHConnRequestRemoveArg) (params.ErrorResult, error) {
	err := f.backend.RemoveSSHConnRequest(state.SSHConnRequestRemoveArg(arg))
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}
