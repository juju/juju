// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/juju/worker/uniter/hook"
)

// HookQueue exists to keep the package interface stable.
type HookQueue interface {
	HookSender
}

// NewAliveHookQueue returns a new HookQueue that aggregates the values
// obtained from the w watcher and sends into out the details about hooks that
// must be executed in the unit. It guarantees that the stream of hooks will
// respect the guarantees Juju makes about hook execution order. If any values
// have previously been received from w's Changes channel, the HookQueue's
// behaviour is undefined.
func NewAliveHookQueue(initial *State, out chan<- hook.Info, w RelationUnitsWatcher) HookQueue {
	source := NewLiveHookSource(initial, w)
	return NewHookSender(out, source)
}

// NewDyingHookQueue returns a new HookQueue that sends all hooks necessary
// to clean up the supplied initial relation hook state, while preserving the
// guarantees Juju makes about hook execution order.
func NewDyingHookQueue(initial *State, out chan<- hook.Info) HookQueue {
	source := NewDyingHookSource(initial)
	return NewHookSender(out, source)
}
