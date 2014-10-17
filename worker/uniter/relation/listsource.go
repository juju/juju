// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/juju/worker/uniter/hook"
)

type listSource struct {
	NoUpdates
	hooks []hook.Info
}

func (q *listSource) Empty() bool {
	return len(q.hooks) == 0
}

func (q *listSource) Next() hook.Info {
	if q.Empty() {
		panic("HookSource is empty")
	}
	return q.hooks[0]
}

func (q *listSource) Pop() {
	if q.Empty() {
		panic("HookSource is empty")
	}
	q.hooks = q.hooks[1:]
}

// NewListSource returns a HookSource that generates only the supplied hooks, in
// order; and which cannot be updated.
func NewListSource(list []hook.Info) HookSource {
	source := &listSource{hooks: make([]hook.Info, len(list))}
	copy(source.hooks, list)
	return source
}
