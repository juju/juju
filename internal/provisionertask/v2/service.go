// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask

import (
	"context"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
)

// MachineAPI provides operations on a machine that the worker needs.
// This interface abstracts the real Juju machine API for testability.
type MachineAPI interface {
	// ID returns the machine's ID.
	ID() string

	// Life returns the machine's current life value.
	Life() life.Value

	// InstanceID returns the instance ID if the machine has been provisioned,
	// or an empty string if not.
	InstanceID() string

	// KeepInstance returns true if the machine should keep its instance
	// when dying/dead (i.e., not call StopInstances).
	KeepInstance() bool

	// EnsureDead ensures the machine is marked as dead in state.
	EnsureDead(ctx context.Context) error

	// SetStatus sets the machine's status.
	SetStatus(ctx context.Context, st status.Status, message string) error

	// SetInstanceStatus sets the machine's instance status.
	SetInstanceStatus(ctx context.Context, st status.Status, message string) error

	// MarkForRemoval marks the machine for removal.
	MarkForRemoval(ctx context.Context) error
}

// InstanceBroker provides provisioning operations.
// This interface abstracts the real environs broker for testability.
type InstanceBroker interface {
	// StartInstance starts a new instance for the given machine.
	// Returns the instance ID and any error that occurred.
	StartInstance(ctx context.Context, params StartInstanceParams) (StartInstanceResult, error)

	// StopInstances stops the given instances.
	StopInstances(ctx context.Context, instanceIDs ...string) error
}

// InstanceInfoSetter provides the operation to register instance info.
type InstanceInfoSetter interface {
	// SetInstanceInfo registers the instance information for a machine.
	SetInstanceInfo(ctx context.Context, machineID, instanceID, zoneName string) error
}

// ProviderSemaphore limits concurrent provider API calls.
type ProviderSemaphore interface {
	// Acquire blocks until a slot is available or context is cancelled.
	// Returns nil on success, or ctx.Err() if cancelled.
	Acquire(ctx context.Context) error

	// Release returns a slot to the pool.
	Release()
}

// Logger provides logging functionality for the worker.
type Logger = logger.Logger
