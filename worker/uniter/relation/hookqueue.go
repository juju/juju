// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

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
	q := &hookSender{
		out: out,
	}
	go func() {
		defer q.tomb.Done()
		source := newLiveSource(initial, w)
		q.tomb.Kill(q.loop(source))
	}()
	return q
}

func NewDyingHookQueue(initial *State, out chan<- hook.Info) HookQueue {
	q := &hookSender{
		out: out,
	}
	go func() {
		defer q.tomb.Done()
		source := newDyingSource(initial)
		q.tomb.Kill(q.loop(source))
	}()
	return q
}
