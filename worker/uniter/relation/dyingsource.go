// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"sort"

	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/hook"
)

// NewDyingHookSource returns a new HookSource that generates all hooks
// necessary to clean up the supplied initial relation hook state, while
// preserving the guarantees Juju makes about hook execution order.
func NewDyingHookSource(initial *State) hook.Source {
	var list []hook.Info

	// Honour any expected relation-changed hook.
	if initial.ChangedPending != "" {
		list = append(list, hook.Info{
			Kind:          hooks.RelationChanged,
			RelationId:    initial.RelationId,
			RemoteUnit:    initial.ChangedPending,
			ChangeVersion: initial.Members[initial.ChangedPending],
		})
	}

	// Depart in consistent order, mainly for testing purposes.
	departs := []string{}
	for name := range initial.Members {
		departs = append(departs, name)
	}
	sort.Strings(departs)
	for _, name := range departs {
		list = append(list, hook.Info{
			Kind:          hooks.RelationDeparted,
			RelationId:    initial.RelationId,
			RemoteUnit:    name,
			ChangeVersion: initial.Members[name],
		})
	}

	// Finally break the relation.
	list = append(list, hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: initial.RelationId,
	})

	return NewListSource(list)
}
