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

// NewAliveHookQueue exists to keep the package interface stable; it wraps the
// result of NewLiveHookSource in a HookSender.
func NewAliveHookQueue(initial *State, out chan<- hook.Info, w RelationUnitsWatcher) HookQueue {
	source := NewLiveHookSource(initial, w)
	return NewHookSender(out, source)
}

// NewDyingHookQueue exists to keep the package interface stable; it wraps the
// result of NewDyingHookSource in a HookSender.
func NewDyingHookQueue(initial *State, out chan<- hook.Info) HookQueue {
	source := NewDyingHookSource(initial)
	return NewHookSender(out, source)
}
