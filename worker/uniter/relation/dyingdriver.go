package relation

import (
	"sort"

	"gopkg.in/juju/charm.v2/hooks"

	"github.com/juju/juju/worker/uniter/hook"
)

type listSource struct {
	hooks []hook.Info
}

func (q *listSource) Empty() bool {
	return len(q.hooks) == 0
}

func (q *listSource) Next() hook.Info {
	return q.hooks[0]
}

func (q *listSource) Pop() {
	q.hooks = q.hooks[1:]
}

func newDyingSource(initial *State) HookSource {
	source := &listSource{}

	//  * Honour any expected relation-changed hook.
	if initial.ChangedPending != "" {
		source.hooks = append(source.hooks, hook.Info{
			Kind:          hooks.RelationChanged,
			RelationId:    initial.RelationId,
			RemoteUnit:    initial.ChangedPending,
			ChangeVersion: initial.Members[initial.ChangedPending],
		})
	}

	// * Depart in consistent order, mainly for testing purposes.
	departs := []string{}
	for name := range initial.Members {
		departs = append(departs, name)
	}
	sort.Strings(departs)
	for _, name := range departs {
		source.hooks = append(source.hooks, hook.Info{
			Kind:          hooks.RelationDeparted,
			RelationId:    initial.RelationId,
			RemoteUnit:    name,
			ChangeVersion: initial.Members[name],
		})
	}

	// * Finally break the relation.
	source.hooks = append(source.hooks, hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: initial.RelationId,
	})

	return source
}
