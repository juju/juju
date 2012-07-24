package relationer

import (
	"fmt"
	"launchpad.net/juju-core/state"
	"sort"
)

// HookInfo holds details about a relation hook that will be required
// for executing it.
type HookInfo struct {
	HookName   string
	RemoteUnit string
	// Members contains a map[string]interface for every remote unit,
	// keyed on unit name.
	Members map[string]map[string]interface{}
}

// HookQueue accepts state.RelationUnitsChange events and converts them
// into HookInfo values.
type HookQueue struct {
	// head and tail are the ends of the queue.
	head, tail *unitInfo
	// inflight is a clone of the most recent value returned from Peek.
	inflight *unitInfo
	// info holds information about every unit known to the queue.
	info map[string]*unitInfo
}

// unitInfo holds unit information for management by HookQueue.
type unitInfo struct {
	unit     string
	version  int
	settings map[string]interface{}
	present  bool
	// hook holds the current idea of the next hook that should
	// be run for the unit, and should be empty if the unit is
	// not queued.
	hook string
	// prev and next define the position in the queue of the
	// unit's next hook, and should be nil if the unit is not
	// queued.
	prev, next *unitInfo
}

// NewHookQueue returns an empty HookQueue.
func NewHookQueue() *HookQueue {
	return &HookQueue{info: map[string]*unitInfo{}}
}

// TODO better doc
func (q *HookQueue) Add(ruc state.RelationUnitsChange) {
	// Enforce consistent addition order, mainly for testing purposes.
	changedUnits := []string{}
	for unit, _ := range ruc.Changed {
		changedUnits = append(changedUnits, unit)
	}
	sort.Strings(changedUnits)

	for _, unit := range changedUnits {
		settings := ruc.Changed[unit]
		info, found := q.info[unit]
		if !found {
			// If not known, add to info and queue a join.
			info = &unitInfo{unit: unit}
			q.info[unit] = info
			q.queue(unit, "joined")
		} else if info.hook == "" {
			// If known, and not already in queue, queue a change.
			q.queue(unit, "changed")
		} else if info.hook == "departed" {
			if settings.Version == info.version {
				q.remove(unit)
			} else {
				q.queue(unit, "changed")
			}
		} // Otherwise, it's already queued for either join or change; ignore.

		// Always update the stored settings.
		info.version = settings.Version
		info.settings = settings.Settings
	}

	for _, unit := range ruc.Departed {
		if hook := q.info[unit].hook; hook == "" {
			q.queue(unit, "departed")
		} else if hook == "changed" {
			q.queue(unit, "departed")
		} else {
			q.remove(unit)
		}
	}
}

// Ready returns true if there is a hook ready to run.
func (q *HookQueue) Ready() bool {
	return q.head != nil || q.inflight != nil
}

// Next returns a HookInfo describing the next hook to run. If no hook
// is ready to run, Next will panic.
func (q *HookQueue) Next() HookInfo {
	if q.inflight == nil {
		if q.head == nil {
			panic("queue is empty")
		}
		if q.head.hook == "joined" {
			q.head.present = true
		}
		info := *q.head
		q.inflight = &info
		q.inflight.settings = nil
		q.remove(info.unit)
		if info.hook == "departed" {
			delete(q.info, info.unit)
		}
	} else if q.inflight.hook != "departed" {
		q.inflight.version = q.info[q.inflight.unit].version
	}
	hi := HookInfo{
		HookName:   q.inflight.hook,
		RemoteUnit: q.inflight.unit,
		Members:    make(map[string]map[string]interface{}),
	}
	for unit, info := range q.info {
		if info.present {
			hi.Members[unit] = info.settings
		}
	}
	return hi
}

func (q *HookQueue) Done() {
	if prev := q.inflight; prev == nil {
		panic("no previously returned hook")
	} else if prev.hook == "joined" {
		prev.hook = "changed"
	} else {
		q.inflight = nil
		if prev.hook != "changed" {
			return
		}
		if queued := q.info[prev.unit]; queued.hook == "changed" {
			if queued.version == prev.version {
				q.remove(prev.unit)
			}
		}
	}
}

// queue places the named unit at the tail of the queue. It will
// panic if the unit is not in q.info.
func (q *HookQueue) queue(unit, hook string) {
	// First make sure it's out of the queue.
	q.remove(unit)

	// Set the unit's next action.
	info := q.info[unit]
	info.hook = hook

	// Place it at the tail.
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

// remove removes the named unit from the queue. It is fine to
// remove a unit that is not in the queue, but it will panic if
// the unit is not in q.info.
func (q *HookQueue) remove(unit string) {
	if q.head == nil {
		// The queue is empty, nothing to do.
		return
	}

	// Get the unit info and clear its next action.
	info := q.info[unit]
	if info.hook == "" {
		// The unit is not in the queue.
		return
	}
	info.hook = ""

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
