// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import "context"

// State describes model-scoped persistence for SSH virtual host keys.
type State interface {
	// GetMachineVirtualHostKeyByMachineName returns the machine virtual host key.
	// The boolean indicates whether a key row exists.
	GetMachineVirtualHostKeyByMachineName(context.Context, string) (string, bool, error)

	// EnsureMachineVirtualHostKeyByMachineName persists the machine virtual host
	// key when missing, otherwise returns the existing persisted key.
	EnsureMachineVirtualHostKeyByMachineName(context.Context, string, int, string) (string, error)

	// GetUnitVirtualHostKeyByUnitName returns the unit virtual host key.
	// The boolean indicates whether a key row exists.
	GetUnitVirtualHostKeyByUnitName(context.Context, string) (string, bool, error)

	// EnsureUnitVirtualHostKeyByUnitName persists the unit virtual host key when
	// missing, otherwise returns the existing persisted key.
	EnsureUnitVirtualHostKeyByUnitName(context.Context, string, int, string) (string, error)

	// GetMachineNameForUnit returns the machine name for an IAAS unit.
	// The boolean indicates whether the unit is machine backed.
	GetMachineNameForUnit(context.Context, string) (string, bool, error)
}
