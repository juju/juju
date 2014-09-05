package relation

import (
	"sort"

	"gopkg.in/juju/charm.v2/hooks"

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

func NewDyingHookSource(initial *State) HookSource {
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
