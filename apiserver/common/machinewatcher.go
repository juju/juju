// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// WatchableMachineService is an interface that defines the methods related to watching
// changes in the machine domain
//
// It contains a subset  of method from [github.com/juju/juju/domain/machine/service.Service],
// dedicated to watch various machine events.
type WatchableMachineService interface {
	// WatchMachineReboot returns a NotifyWatcher that is subscribed to
	// the changes in the machine_requires_reboot table in the model.
	// It raises an event whenever the machine uuid or its parent is added to the reboot table.
	WatchMachineReboot(ctx context.Context, uuid coremachine.UUID) (watcher.NotifyWatcher, error)
}

// MachineWatcher is a struct that represents a watcher for various events produced by a specific
// machine. It uses the WatchableMachineService interface to watch for changes and the facade.WatcherRegistry
// interface to register and unregister watchers. The GetMachineUUID type is a function that returns the
// UUID of the machine.
type MachineWatcher struct {
	service         WatchableMachineService
	watcherRegistry facade.WatcherRegistry
	machineUUID     GetMachineUUID
}

// GetMachineUUID represents a function type that takes a context.Context as input and returns
// a string representation of the machine UUID and an error. It allows to smuggle machine
// identification to the watcher, because retrieving the machine UUID implies a machine service calls
// which requires a context.
type GetMachineUUID func(context.Context) (coremachine.UUID, error)

// NewMachineRebootWatcher creates a new MachineWatcher instance with the given dependencies.
// It takes a WatchableMachineService, a facade.WatcherRegistry, and a GetMachineUUID function as input.
// The returned MachineWatcher instance tracks changes in the machine domain and registers/unregisters watchers
// for a specific machine UUID.
func NewMachineRebootWatcher(service WatchableMachineService, watcherRegistry facade.WatcherRegistry, uuid GetMachineUUID) *MachineWatcher {
	return &MachineWatcher{
		service:         service,
		watcherRegistry: watcherRegistry,
		machineUUID:     uuid,
	}
}

// WatchForRebootEvent starts a watcher to track if there is a new
// reboot request for a specific machine ID or its parent (in case we are a container).
func (mrw *MachineWatcher) WatchForRebootEvent(ctx context.Context) (params.NotifyWatchResult, error) {
	var result params.NotifyWatchResult

	uuid, err := mrw.machineUUID(ctx)
	if err != nil {
		return params.NotifyWatchResult{}, errors.Trace(err)
	}
	notifyWatcher, err := mrw.service.WatchMachineReboot(ctx, uuid)

	if err != nil {
		return result, errors.Trace(err)
	}
	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, mrw.watcherRegistry, notifyWatcher)
	return result, err
}
