package relation

import (
	"sort"

	"gopkg.in/juju/charm.v2/hooks"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/worker/uniter/hook"
)

func NewDyingHookQueue(initial *State, out chan<- hook.Info) HookQueue {
	q := &hookQueue{
		out: out,
	}
	go func() {
		defer q.tomb.Done()
		q.driver = newDyingDriver(initial)
		q.tomb.Kill(q.loop(nil))
	}()
	return q
}

type dyingDriver struct {
	infos []hook.Info
}

func newDyingDriver(initial *State) queueDriver {
	q := &dyingDriver{}

	// Queue hooks to:
	//  * Honour any expected relation-changed hook.
	if initial.ChangedPending != "" {
		q.infos = append(q.infos, hook.Info{
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
		q.infos = append(q.infos, hook.Info{
			Kind:          hooks.RelationDeparted,
			RelationId:    initial.RelationId,
			RemoteUnit:    name,
			ChangeVersion: initial.Members[name],
		})
	}

	// * Finally break the relation.
	q.infos = append(q.infos, hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: initial.RelationId,
	})

	return q
}

func (q *dyingDriver) update(_ params.RelationUnitsChange) {
	panic("it's not meaningful to update a DyingHookQueue")
}

func (q *dyingDriver) empty() bool {
	return len(q.infos) == 0
}

func (q *dyingDriver) next() hook.Info {
	return q.infos[0]
}

func (q *dyingDriver) pop() {
	q.infos = q.infos[1:]
}
