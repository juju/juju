package relationer

import (
	"launchpad.net/juju-core/state"
	"sort"
)

// HookInfo holds details about a relation hook that will be required
// for executing it.
type HookInfo struct {
	HookKind   string
	RemoteUnit string
	// Members contains a map[string]interface{} for every remote unit,
	// holding its relation settings, keyed on unit name.
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
	// unit holds the name of the unit.
	unit string
	// version and settings hold the most recent settings known
	// to the HookQueue.
	version  int
	settings map[string]interface{}
	// present holds the current idea of whether the unit is a
	// member in the relation, at the point in the history of
	// the relation corresponding to the most recent call to
	// Next.
	present bool
	// hook holds the current idea of the next hook that should
	// be run for the unit, and is empty if and only if the unit
	// is not queued.
	hook string
	// prev and next define the position in the queue of the
	// unit's next hook.
	prev, next *unitInfo
}

// NewHookQueue returns an empty HookQueue.
func NewHookQueue() *HookQueue {
	return &HookQueue{info: map[string]*unitInfo{}}
}

// Add updates the queue such that the stream of HookInfos returned from
// next will reflect the state of the relation according to the supplied
// RelationUnitsChange.
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
			q.add(unit, "joined")
		} else if info.hook == "" {
			// If known, and not already in queue, queue a change.
			q.add(unit, "changed")
		} else if info.hook == "departed" {
			if settings.Version == info.version {
				q.remove(unit)
			} else {
				q.add(unit, "changed")
			}
		} // Otherwise, it's already queued for either join or change; ignore.

		// Always update the stored settings.
		info.version = settings.Version
		info.settings = settings.Settings
	}

	for _, unit := range ruc.Departed {
		if hook := q.info[unit].hook; hook == "" {
			q.add(unit, "departed")
		} else if hook == "changed" {
			q.add(unit, "departed")
		} else {
			q.remove(unit)
		}
	}
}

// Ready returns true if there is a hook ready to run.
func (q *HookQueue) Ready() bool {
	return q.head != nil || q.inflight != nil
}

// Next returns a HookInfo describing the next hook to run. Subsequent
// calls to Next will return the same HookInfo, differing only in the
// settings of Members (if they have been updated), until Done is called.
// Next will panic if it cannot return a value.
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
		q.remove(info.unit)
		if info.hook == "departed" {
			delete(q.info, info.unit)
		}
	} else if q.inflight.hook != "departed" {
		latest := q.info[q.inflight.unit]
		q.inflight.version = latest.version
		q.inflight.settings = latest.settings
	}
	hi := HookInfo{
		HookKind:   q.inflight.hook,
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

// Done signals that the hook returned most recently from Next has
// been fully handled, and should be completely forgotten. It will
// panic if Next has not been called, or has not been called since
// the previous call to Done.
func (q *HookQueue) Done() {
	if prev := q.inflight; prev == nil {
		panic("no inflight hook")
	} else if prev.hook == "joined" {
		prev.hook = "changed"
	} else {
		q.inflight = nil
		if prev.hook != "changed" {
			return
		}
		// If we have a redundant hook queued, we can remove it.
		if queued := q.info[prev.unit]; queued.hook == "changed" {
			if queued.version == prev.version {
				q.remove(prev.unit)
			}
		}
	}
}

// add sets the next hook to be run for the named unit, and places it
// at the tail of the queue if it is not already queued. It will panic
// if the unit is not in q.info.
func (q *HookQueue) add(unit, hook string) {
	// If the unit is not in the queue, place it at the tail.
	info := q.info[unit]
	if info.hook == "" {
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
	info.hook = hook
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
		// The unit is not in the queue, nothing to do.
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
