// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	domainssh "github.com/juju/juju/domain/ssh"
)

// State describes model-scoped persistence for SSH virtual host keys and SSH
// connection requests.
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

	// InsertSSHConnRequest persists a one-shot SSH connection request.
	InsertSSHConnRequest(context.Context, domainssh.SSHConnRequest, time.Time) error

	// GetSSHConnRequest returns the one-shot SSH connection request for a tunnel
	// ID, scoped to the named machine, pruning expired requests first. A request
	// targeting another machine is reported as not found.
	GetSSHConnRequest(context.Context, string, string, time.Time) (domainssh.SSHConnRequest, error)

	// RemoveSSHConnRequest deletes the request for the supplied tunnel ID.
	RemoveSSHConnRequest(context.Context, string) error

	// PruneExpiredSSHConnRequests removes expired SSH connection requests.
	PruneExpiredSSHConnRequests(context.Context, time.Time) error

	// GetMachineUUIDByName returns the UUID of the named machine.
	GetMachineUUIDByName(context.Context, string) (string, error)

	// InitialWatchSSHConnRequestsStatement returns the changelog namespace and
	// initial state statement for a machine's SSH connection request watcher.
	// The initial statement is parameterised by the machine UUID so that the
	// watcher only reports the machine's own requests.
	InitialWatchSSHConnRequestsStatement() (string, string)

	// FilterSSHConnRequestsForMachine returns the subset of the supplied tunnel
	// IDs that identify SSH connection requests targeting the given machine.
	FilterSSHConnRequestsForMachine(context.Context, []string, string) ([]string, error)
}
