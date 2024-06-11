// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Machiner represents necessary methods for this worker from the
// machiner api.
type Machiner interface {
	Machine(ctx context.Context, tag names.MachineTag) (Machine, error)
}

// Machine represents necessary methods for this worker from the
// a machiner's machine.
type Machine interface {
	Jobs() (*params.JobsResult, error)
	Watch(context.Context) (watcher.NotifyWatcher, error)
}
