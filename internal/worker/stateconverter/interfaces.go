// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
)

// Machiner represents necessary methods for this worker from the
// machiner api.
type Machiner interface {
	Machine(ctx context.Context, tag names.MachineTag) (Machine, error)
}

// Machine represents necessary methods for this worker from the
// a machiner's machine.
type Machine interface {
	IsController(context.Context, string) (bool, error)
	Watch(context.Context) (watcher.NotifyWatcher, error)
}

// Agent represents the necessary methods for this worker from the
// agent api.
type Agent interface {
	StateServingInfo(ctx context.Context) (controller.StateServingInfo, error)
}
