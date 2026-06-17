// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import "context"

// State describes model-scoped persistence for SSH virtual host keys.
type State interface {
	// GetMachineVirtualHostKeyByMachineName returns the machine virtual host key.
	// The boolean indicates whether a key row exists.
	GetMachineVirtualHostKeyByMachineName(context.Context, string) (string, bool, error)

	// SetMachineVirtualHostKeyByMachineName persists the machine virtual host key.
	SetMachineVirtualHostKeyByMachineName(context.Context, string, int, string) error

	// GetUnitVirtualHostKeyByUnitName returns the unit virtual host key.
	// The boolean indicates whether a key row exists.
	GetUnitVirtualHostKeyByUnitName(context.Context, string) (string, bool, error)

	// SetUnitVirtualHostKeyByUnitName persists the unit virtual host key.
	SetUnitVirtualHostKeyByUnitName(context.Context, string, int, string) error

	// GetMachineNameForUnit returns the machine name for an IAAS unit.
	// The boolean indicates whether the unit is machine backed.
	GetMachineNameForUnit(context.Context, string) (string, bool, error)
}
