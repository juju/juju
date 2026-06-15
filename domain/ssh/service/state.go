// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
)

// ControllerState describes controller-scoped persistence for SSH host keys.
type ControllerState interface {
	// GetSSHServerHostKey returns the stored controller jump host key.
	// The boolean indicates whether a key row exists.
	GetSSHServerHostKey(context.Context) (string, bool, error)
}

// ModelState describes model-scoped persistence for SSH virtual host keys.
type ModelState interface {
	// GetMachineVirtualHostKeyByMachineName returns the machine virtual host key.
	// The boolean indicates whether a key row exists.
	GetMachineVirtualHostKeyByMachineName(context.Context, string) (string, bool, error)

	// SetMachineVirtualHostKeyByMachineName persists the machine virtual host key.
	SetMachineVirtualHostKeyByMachineName(context.Context, string, string) error

	// GetUnitVirtualHostKeyByUnitName returns the unit virtual host key.
	// The boolean indicates whether a key row exists.
	GetUnitVirtualHostKeyByUnitName(context.Context, string) (string, bool, error)

	// SetUnitVirtualHostKeyByUnitName persists the unit virtual host key.
	SetUnitVirtualHostKeyByUnitName(context.Context, string, string) error

	// GetMachineNameForUnit returns the machine name for an IAAS unit.
	// The boolean indicates whether the unit is machine backed.
	GetMachineNameForUnit(context.Context, string) (string, bool, error)
}

// ModelStateGetter returns model-scoped SSH state for a model.
type ModelStateGetter interface {
	GetModelState(coremodel.UUID) ModelState
}

// ModelStateGetterFunc adapts a function to a [ModelStateGetter].
type ModelStateGetterFunc func(coremodel.UUID) ModelState

// GetModelState implements [ModelStateGetter].
func (f ModelStateGetterFunc) GetModelState(modelUUID coremodel.UUID) ModelState {
	return f(modelUUID)
}
