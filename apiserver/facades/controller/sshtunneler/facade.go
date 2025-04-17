// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	stderr "errors"

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
	ControllerMachine(machineID string) (*state.Machine, error)
	SSHHostKeys(modelUUID string, machineTag names.MachineTag) (state.SSHHostKeys, error)
}

// Facade is the interface exposing the SSHTunneler methods.
type Facade struct {
	backend Backend
}

// newFacade creates the facade for the SSHTunneler.
func newFacade(_ facade.Context, backend Backend) *Facade {
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
func (f *Facade) ControllerAddresses(et params.Entity) params.StringsResult {
	tag, err := names.ParseTag(et.Tag)
	if err != nil {
		return params.StringsResult{Error: apiservererrors.ServerError(err)}
	}

	// We expect a controller tag for controllers bootstrapped on K8s
	// and a machine tag for controllers bootstrapped on machines.
	var result params.StringsResult
	switch tag.Kind() {
	case names.ControllerTagKind:
		// TODO (JUJU-7887): Support SSH from machines to K8s controller.
		result.Error = apiservererrors.ServerError(stderr.New("SSH proxy from machine to k8s controller not supported"))
	case names.MachineTagKind:
		m, err := f.backend.ControllerMachine(tag.Id())
		if err != nil {
			return params.StringsResult{Error: apiservererrors.ServerError(err)}
		}
		result.Result = append(result.Result, m.Addresses().AllMatchingScope(network.ScopeMatchPublic).Values()...)
	}

	return result
}

// MachineHostKeys returns the host keys for a specified machine.
func (f *Facade) MachineHostKeys(machine params.SSHMachineHostKeysArg) (params.SSHPublicKeysResult, error) {
	var result params.SSHPublicKeysResult

	mt, err := names.ParseMachineTag(machine.MachineTag)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}

	keys, err := f.backend.SSHHostKeys(machine.ModelUUID, mt)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}

	result.PublicKeys = keys
	return result, nil
}
