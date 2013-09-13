// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"sort"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/charm/hooks"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/worker/uniter/hook"
)

// HookQueue is the minimal interface implemented by both AliveHookQueue and
// DyingHookQueue.
type HookQueue interface {
	hookQueue()
	Stop() error
}

// RelationUnitsWatcher is used to enable deterministic testing of
// AliveHookQueue, by supplying a reliable stream of RelationUnitsChange
// events; usually, it will be a *state.RelationUnitsWatcher.
type RelationUnitsWatcher interface {
	Err() error
	Stop() error
	Changes() <-chan params.RelationUnitsChange
}

// AliveHookQueue aggregates values obtained from a relation units watcher
// and sends out details about hooks that must be executed in the unit.
type AliveHookQueue struct {
	tomb       tomb.Tomb
	w          RelationUnitsWatcher
	out        chan<- hook.Info
	relationId int

	// info holds information about all units that were added to the
	// queue and haven't had a "relation-departed" event popped. This
	// means the unit may be in info and not currently in the queue
	// itself.
	info map[string]*unitInfo

	// head and tail are the ends of the queue.
	head, tail *unitInfo

	// changedPending, if not empty, indicates that the most recently
	// popped event was a "relation-joined" for the named unit, and
	// therefore that the next event must be a "relation-changed"
	// for that same unit.
	// If changedPending is not empty, the queue is considered non-
	// empty, even if head is nil.
	changedPending string
}

// unitInfo holds unit information for management by AliveHookQueue.
type unitInfo struct {
	// unit holds the name of the unit.
	unit string

	// version and settings hold the most recent settings known
	// to the AliveHookQueue.
	version int64

	// joined is set to true when a "relation-joined" is popped for this unit.
	joined bool

	// hookKind holds the current idea of the next hook that should
	// be run for the unit, and is empty if and only if the unit
	// is not queued.
	hookKind hooks.Kind

	// prev and next define the position in the queue of the
	// unit's next hook.
	prev, next *unitInfo
}

// NewAliveHookQueue returns a new AliveHookQueue that aggregates the values
// obtained from the w watcher and sends into out the details about hooks that
// must be executed in the unit. It guarantees that the stream of hooks will
// respect the guarantees Juju makes about hook execution order. If any values
// have previously been received from w's Changes channel, the AliveHookQueue's
// behaviour is undefined.
func NewAliveHookQueue(initial *State, out chan<- hook.Info, w RelationUnitsWatcher) *AliveHookQueue {
	q := &AliveHookQueue{
		w:          w,
		out:        out,
		relationId: initial.RelationId,
		info:       map[string]*unitInfo{},
	}
	go q.loop(initial)
	return q
}

func (q *AliveHookQueue) loop(initial *State) {
	defer q.tomb.Done()
	defer watcher.Stop(q.w, &q.tomb)

	// Consume initial event, and reconcile with initial state, by inserting
	// a new RelationUnitsChange before the initial event, which schedules
	// every missing unit for immediate departure before anything else happens
	// (apart from a single potential required post-joined changed event).
	ch1, ok := <-q.w.Changes()
	if !ok {
		q.tomb.Kill(watcher.MustErr(q.w))
		return
	}
	if len(ch1.Departed) != 0 {
		panic("AliveHookQueue must be started with a fresh RelationUnitsWatcher")
	}
	q.changedPending = initial.ChangedPending
	ch0 := params.RelationUnitsChange{}
	for unit, version := range initial.Members {
		q.info[unit] = &unitInfo{
			unit:    unit,
			version: version,
			joined:  true,
		}
		if _, found := ch1.Changed[unit]; !found {
			ch0.Departed = append(ch0.Departed, unit)
		}
	}
	q.update(ch0)
	q.update(ch1)

	var next hook.Info
	var out chan<- hook.Info
	for {
		if q.empty() {
			out = nil
		} else {
			out = q.out
			next = q.next()
		}
		select {
		case <-q.tomb.Dying():
			return
		case ch, ok := <-q.w.Changes():
			if !ok {
				q.tomb.Kill(watcher.MustErr(q.w))
				return
			}
			q.update(ch)
		case out <- next:
			q.pop()
		}
	}
}

func (q *AliveHookQueue) hookQueue() {
	panic("interface sentinel method, do not call")
}

// Stop stops the AliveHookQueue and returns any errors encountered during
// operation or while shutting down.
func (q *AliveHookQueue) Stop() error {
	q.tomb.Kill(nil)
	return q.tomb.Wait()
}

// empty returns true if the queue is empty.
func (q *AliveHookQueue) empty() bool {
	return q.head == nil && q.changedPending == ""
}

// update modifies the queue such that the hook.Info values it sends will
// reflect the supplied change.
func (q *AliveHookQueue) update(ruc params.RelationUnitsChange) {
	// Enforce consistent addition order, mainly for testing purposes.
	changedUnits := []string{}
	for unit := range ruc.Changed {
		changedUnits = append(changedUnits, unit)
	}
	sort.Strings(changedUnits)

	for _, unit := range changedUnits {
		settings := ruc.Changed[unit]
		info, found := q.info[unit]
		if !found {
			info = &unitInfo{unit: unit}
			q.info[unit] = info
			q.queue(unit, hooks.RelationJoined)
		} else if info.hookKind != hooks.RelationJoined {
			if settings.Version != info.version {
				q.queue(unit, hooks.RelationChanged)
			} else {
				q.unqueue(unit)
			}
		}
		info.version = settings.Version
	}

	for _, unit := range ruc.Departed {
		if q.info[unit].hookKind == hooks.RelationJoined {
			q.unqueue(unit)
		} else {
			q.queue(unit, hooks.RelationDeparted)
		}
	}
}

// pop advances the queue. It will panic if the queue is already empty.
func (q *AliveHookQueue) pop() {
	if q.empty() {
		panic("queue is empty")
	}
	if q.changedPending != "" {
		if q.info[q.changedPending].hookKind == hooks.RelationChanged {
			// We just ran this very hook; no sense keeping it queued.
			q.unqueue(q.changedPending)
		}
		q.changedPending = ""
	} else {
		old := *q.head
		q.unqueue(q.head.unit)
		if old.hookKind == hooks.RelationJoined {
			q.changedPending = old.unit
			q.info[old.unit].joined = true
		} else if old.hookKind == hooks.RelationDeparted {
			delete(q.info, old.unit)
		}
	}
}

// next returns the next hook.Info value to send.
func (q *AliveHookQueue) next() hook.Info {
	if q.empty() {
		panic("queue is empty")
	}
	var unit string
	var kind hooks.Kind
	if q.changedPending != "" {
		unit = q.changedPending
		kind = hooks.RelationChanged
	} else {
		unit = q.head.unit
		kind = q.head.hookKind
	}
	version := q.info[unit].version
	return hook.Info{
		Kind:          kind,
		RelationId:    q.relationId,
		RemoteUnit:    unit,
		ChangeVersion: version,
	}
}

// queue sets the next hook to be run for the named unit, and places it
// at the tail of the queue if it is not already queued. It will panic
// if the unit is not in q.info.
func (q *AliveHookQueue) queue(unit string, kind hooks.Kind) {
	// If the unit is not in the queue, place it at the tail.
	info := q.info[unit]
	if info.hookKind == "" {
		info.prev = q.tail
		if q.tail != nil {
			q.tail.next = info
		}
		q.tail = info

		// If the queue is empty, the tail is also the head.
		if q.head == nil {
			q.head = info
		}
	}
	info.hookKind = kind
}

// unqueue removes the named unit from the queue. It is fine to
// unqueue a unit that is not in the queue, but it will panic if
// the unit is not in q.info.
func (q *AliveHookQueue) unqueue(unit string) {
	if q.head == nil {
		// The queue is empty, nothing to do.
		return
	}

	// Get the unit info and clear its next action.
	info := q.info[unit]
	if info.hookKind == "" {
		// The unit is not in the queue, nothing to do.
		return
	}
	info.hookKind = ""

	// Update queue pointers.
	if info.prev == nil {
		q.head = info.next
	} else {
		info.prev.next = info.next
	}
	if info.next == nil {
		q.tail = info.prev
	} else {
		info.next.prev = info.prev
	}
	info.prev = nil
	info.next = nil
}

// DyingHookQueue is a hook queue that deals with a relation that is being
// shut down. It honours the obligations of an AliveHookQueue with respect to
// relation hook execution order; as soon as those obligations are fulfilled,
// it sends a "relation-departed" hook for every relation member, and finally a
// "relation-broken" hook for the relation itself.
type DyingHookQueue struct {
	tomb           tomb.Tomb
	out            chan<- hook.Info
	relationId     int
	members        map[string]int64
	changedPending string
}

// NewDyingHookQueue returns a new DyingHookQueue that shuts down the state in
// initial.
func NewDyingHookQueue(initial *State, out chan<- hook.Info) *DyingHookQueue {
	q := &DyingHookQueue{
		out:            out,
		relationId:     initial.RelationId,
		members:        map[string]int64{},
		changedPending: initial.ChangedPending,
	}
	for m, v := range initial.Members {
		q.members[m] = v
	}
	go q.loop()
	return q
}

func (q *DyingHookQueue) loop() {
	defer q.tomb.Done()

	// Honour any expected relation-changed hook.
	if q.changedPending != "" {
		select {
		case <-q.tomb.Dying():
			return
		case q.out <- q.hookInfo(hooks.RelationChanged, q.changedPending):
		}
	}

	// Depart in consistent order, mainly for testing purposes.
	departs := []string{}
	for m := range q.members {
		departs = append(departs, m)
	}
	sort.Strings(departs)
	for _, unit := range departs {
		select {
		case <-q.tomb.Dying():
			return
		case q.out <- q.hookInfo(hooks.RelationDeparted, unit):
		}
	}

	// Finally break the relation.
	select {
	case <-q.tomb.Dying():
		return
	case q.out <- hook.Info{Kind: hooks.RelationBroken, RelationId: q.relationId}:
	}
	q.tomb.Kill(nil)
	return
}

// hookInfo updates the queue's internal membership state according to the
// supplied information, and returns a hook.Info reflecting that change.
func (q *DyingHookQueue) hookInfo(kind hooks.Kind, unit string) hook.Info {
	hi := hook.Info{
		Kind:          kind,
		RelationId:    q.relationId,
		RemoteUnit:    unit,
		ChangeVersion: q.members[unit],
	}
	return hi
}

func (q *DyingHookQueue) hookQueue() {
	panic("interface sentinel method, do not call")
}

// Stop stops the DyingHookQueue and returns any errors encountered
// during operation or while shutting down.
func (q *DyingHookQueue) Stop() error {
	q.tomb.Kill(nil)
	return q.tomb.Wait()
}
