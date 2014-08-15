// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"sort"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v2/hooks"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/worker/uniter/hook"
)

// unitInfo holds unit information for management by liveSource.
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

type liveSource struct {
	watcher    RelationUnitsWatcher
	started    bool
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

func newLiveSource(initial *State, w RelationUnitsWatcher) HookSource {
	info := map[string]*unitInfo{}
	for unit, version := range initial.Members {
		info[unit] = &unitInfo{
			unit:    unit,
			version: version,
			joined:  true,
		}
	}
	return &liveSource{
		watcher:        w,
		info:           info,
		relationId:     initial.RelationId,
		changedPending: initial.ChangedPending,
	}
}

// Empty returns true if the queue is empty.
func (q *liveSource) Empty() bool {
	if !q.started {
		return true
	}
	return q.head == nil && q.changedPending == ""
}

// Next returns the next hook.Info value to send. It will panic if the queue is
// empty.
func (q *liveSource) Next() hook.Info {
	if q.Empty() {
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

// Pop advances the queue. It will panic if the queue is already empty.
func (q *liveSource) Pop() {
	if q.Empty() {
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

// Update modifies the queue such that the hook.Info values it sends will
// reflect the supplied change.
func (q *liveSource) Update(ruc params.RelationUnitsChange) error {
	if !q.started {
		// The first event represents the ideal final state of the system.
		// If it contains any Departed notifications, it cannot be one of
		// those -- most likely the watcher was not a fresh one -- and we're
		// completely hosed.
		if len(ruc.Departed) != 0 {
			return errors.New("hook source watcher sent bad event")
		}
		// Anyway, before we can generate actual hooks, we have to reconcile
		// with initial state to ensure that we generate departed hooks for
		// any previously-known members not reflected in the ideal state.
		departs := params.RelationUnitsChange{}
		for unit := range q.info {
			if _, found := ruc.Changed[unit]; !found {
				departs.Departed = append(departs.Departed, unit)
			}
		}
		q.update(departs)
		q.started = true
	}
	q.update(ruc)
	return nil
}

func (q *liveSource) update(ruc params.RelationUnitsChange) {
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

// queue sets the next hook to be run for the named unit, and places it
// at the tail of the queue if it is not already queued. It will panic
// if the unit is not in q.info.
func (q *liveSource) queue(unit string, kind hooks.Kind) {
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
func (q *liveSource) unqueue(unit string) {
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
