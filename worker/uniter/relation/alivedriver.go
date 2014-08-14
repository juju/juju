// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"sort"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v2/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker/uniter/hook"
)

// RelationUnitsWatcher produces RelationUnitsChange events until stopped, or
// until it encounters an error. It must not close its Changes channel without
// signalling an error via Stop and Err.
type RelationUnitsWatcher interface {
	Err() error
	Stop() error
	Changes() <-chan params.RelationUnitsChange
}

// NewAliveHookQueue returns a new HookQueue that aggregates the values
// obtained from the w watcher and sends into out the details about hooks that
// must be executed in the unit. It guarantees that the stream of hooks will
// respect the guarantees Juju makes about hook execution order. If any values
// have previously been received from w's Changes channel, the HookQueue's
// behaviour is undefined.
func NewAliveHookQueue(initial *State, out chan<- hook.Info, w RelationUnitsWatcher) HookQueue {
	q := &hookQueue{
		out: out,
	}
	go func() {
		defer q.tomb.Done()
		defer watcher.Stop(w, &q.tomb)
		q.tomb.Kill(runAliveHookQueue(q, w, initial))

	}()
	return q
}

var errBadWatcher = errors.New("hook queue watcher sent bad event")
var errWatcherStopped = errors.New("hook queue watcher stopped unexpectedly")

// runAliveHookQueue consumes the watcher's initial event and uses it to
// initalise the driver; it runs the hookQueue; and it handles watcher-related
// errors throughout.
func runAliveHookQueue(q *hookQueue, w RelationUnitsWatcher, initial *State) (err error) {
	defer func() {
		if err == errWatcherStopped {
			// TODO(fwereade): eliminate MustErr, it's stupid
			err = watcher.MustErr(w)
		}
	}()
	select {
	case <-q.tomb.Dying():
		return tomb.ErrDying
	case ideal, ok := <-w.Changes():
		if !ok {
			return errWatcherStopped
		}
		if len(ideal.Departed) != 0 {
			return errBadWatcher
		}
		q.driver = newAliveDriver(initial, ideal)
		return q.loop(w.Changes())
	}
}

// unitInfo holds unit information for management by aliveDriver.
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

type aliveDriver struct {
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

func newAliveDriver(initial *State, ideal params.RelationUnitsChange) queueDriver {
	q := &aliveDriver{
		info:           map[string]*unitInfo{},
		relationId:     initial.RelationId,
		changedPending: initial.ChangedPending,
	}

	// While filling in q.info from initial, check for members not present in
	// the ideal state, and craft an event to insert Departeds for each such
	// unit *before* updating with the ideal state, with the effect that the
	// hooks for those removed units are left at the head of the queue.
	departs := params.RelationUnitsChange{}
	for unit, version := range initial.Members {
		q.info[unit] = &unitInfo{
			unit:    unit,
			version: version,
			joined:  true,
		}
		if _, found := ideal.Changed[unit]; !found {
			departs.Departed = append(departs.Departed, unit)
		}
	}
	q.update(departs)
	q.update(ideal)
	return q
}

// empty returns true if the queue is empty.
func (q *aliveDriver) empty() bool {
	return q.head == nil && q.changedPending == ""
}

// update modifies the queue such that the hook.Info values it sends will
// reflect the supplied change.
func (q *aliveDriver) update(ruc params.RelationUnitsChange) {
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
func (q *aliveDriver) pop() {
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
func (q *aliveDriver) next() hook.Info {
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
func (q *aliveDriver) queue(unit string, kind hooks.Kind) {
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
func (q *aliveDriver) unqueue(unit string) {
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
