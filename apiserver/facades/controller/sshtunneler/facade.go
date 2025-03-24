// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// Backend provides required state for the Facade.
type Backend interface {
	InsertSSHConnRequest(arg state.SSHConnRequestArg) error
	RemoveSSHConnRequest(arg state.SSHConnRequestRemoveArg) error
	Machine(id string) (*state.Machine, error)
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

// ControllerAddresses returns the specified machine's public addresses.
func (f *Facade) ControllerAddresses(machine params.Entity) (params.StringsResult, error) {
	mt, err := names.ParseMachineTag(machine.Tag)
	if err != nil {
		return params.StringsResult{}, err
	}
	m, err := f.backend.Machine(mt.Id())
	if err != nil {
		return params.StringsResult{}, err
	}
	var result params.StringsResult
	result.Result = append(result.Result, m.Addresses().AllMatchingScope(network.ScopeMatchPublic).Values()...)
	return result, nil
}
