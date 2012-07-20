package relationer

import (
	"launchpad.net/juju-core/state"
	"sort"
)

// HookInfo holds everything we need to know to execute a relation hook,
// assuming we have access to the relation itself and the unit executing
// the hook.
type HookInfo struct {
	HookName   string
	RemoteUnit string
	Members    map[string]map[string]interface{}
}

// unitInfo holds information about a unit that is, or has been, or will
// be, a member of the relation; and the next hook that will run and its
// position in the queue if appropriate.
type unitInfo struct {
	unit     string
	version  int
	settings map[string]interface{}
	hook     string
	prev     *unitInfo
	next     *unitInfo
}

// HookQueue accepts state.RelationUnitsChange events and converts them
// into HookInfos.
type HookQueue struct {
	// info holds the unitInfo for every unit known to be present in
	// the relation at any time from the "present" to the furthest
	// known "future".
	info map[string]*unitInfo

	// head and tail are the ends of the queue.
	head *unitInfo
	tail *unitInfo

	// members holds the names of the relation units known to the last
	// HookInfo emitted from the queue; from the point of view of the
	// unit agent, it is the "present" membership of the relation.
	members map[string]struct{}

	// inflight holds enough information to reconstruct the last HookInfo
	// returned from Next, until Done is called.
	inflight *unitInfo
}

// NewHookQueue returns an empty HookQueue.
func NewHookQueue() *HookQueue {
	return &HookQueue{
		members: map[string]struct{}{},
		info:    map[string]*unitInfo{},
	}
}

// Add updates the queue.
func (q *HookQueue) Add(ruc state.RelationUnitsChange) {
	// Enforce consistent addition order, mainly for testing purposes.
	changedUnits := []string{}
	for unit, _ := range ruc.Changed {
		changedUnits = append(changedUnits, unit)
	}
	sort.Strings(changedUnits)

	for _, unit := range changedUnits {
		settings := ruc.Changed[unit]
		if info, found := q.info[unit]; !found {
			// If not known, add to info and queue a join.
			q.info[unit] = &unitInfo{unit: unit}
			q.append(unit, "joined")
		} else if info.hook == "" {
			// If known, and not already in queue, queue a change.
			q.append(unit, "changed")
		} else if info.hook == "departed" {
			// If known, and queued for depart, clear out the old entry...
			q.remove(unit)
			if settings.Version != info.version {
				// ...and queue a change if justified.
				q.append(unit, "changed")
			}
		} // Otherwise, it's already queued for either join or change; ignore.

		// Always update the stored settings.
		q.info[unit].version = settings.Version
		q.info[unit].settings = settings.Settings
	}

	for _, unit := range ruc.Departed {
		if hook := q.info[unit].hook; hook == "" {
			// If not present in queue, queue for depart.
			q.append(unit, "departed")
		} else {
			// Otherwise, clear out the old entry...
			q.remove(unit)
			if hook == "changed" {
				// ...and schedule a deletion if justified.
				q.append(unit, "departed")
			}
		}
	}
}

// Next returns information about the next hook to fire. If the returned
// bool is false, the HookInfo is not valid and the queue is currently empty.
// If the HookInfo is valid, subsequent calls to Next will return the same
// HookInfo, until Done is called.
func (q *HookQueue) Next() (HookInfo, bool) {
	if q.inflight == nil {
		if q.head == nil {
			// The queue is empty.
			return HookInfo{}, false
		}
		// Clone the unit info and update our state.
		info := *q.head
		q.inflight = &info
		if info.hook == "joined" {
			q.members[info.unit] = struct{}{}
			q.head.hook = "changed"
		} else {
			q.remove(info.unit)
			if info.hook == "departed" {
				delete(q.info, info.unit)
				delete(q.members, info.unit)
			}
		}
	} else if q.inflight.hook == "changed" {
		// It may be the case that the settings for an inflight changed
		// hook have changed since we last tried to run it. If that is
		// so, *and* that unit has a queued changed hook, that queued hook
		// is now redundant and can be dropped from the queue. (If the
		// settings have changed, we will certainly have a queued hook
		// for that unit; but the unit might subsequently have departed,
		// so it's not necessarily a changed hook.)
		latest := q.info[q.inflight.unit]
		if q.inflight.version != latest.version && latest.hook == "changed" {
			q.remove(latest.unit)
		}
	}

	// Return a HookInfo created from the inflight unitInfo.
	result := HookInfo{
		HookName:   q.inflight.hook,
		RemoteUnit: q.inflight.unit,
		Members:    make(map[string]map[string]interface{}),
	}
	for m := range q.members {
		result.Members[m] = q.info[m].settings
	}
	return result, true
}

// Done informs the queue that the last HookInfo returned from Next has been
// handled, and can be safely forgotten. Done will panic if no change is
// in flight.
func (q *HookQueue) Done() {
	if q.inflight == nil {
		panic("can't call Done when no hook is inflight")
	}
	q.inflight = nil
}

// append places the named unit info at the tail of the queue. It will
// panic if the unit is not known.
func (q *HookQueue) append(unit, hook string) {
	// First make sure it's out of the queue.
	q.remove(unit)

	// Set the unit's next action.
	info := q.info[unit]
	info.hook = hook

	// If the queue is empty, place it at the start.
	if q.head == nil {
		q.head = info
	}

	// Place it at the end.
	info.prev = q.tail
	if q.tail != nil {
		q.tail.next = info
	}
	q.tail = info
}

// remove removes the named unit from the queue, but does not remove
// its information. It will panic if the unit is not known.
func (q *HookQueue) remove(unit string) {
	if q.head == nil {
		// The queue is empty, nothing to do.
		return
	}

	// Get the unit info and clear its next action.
	info := q.info[unit]
	info.hook = ""

	// If the info is at either end (or both ends) of the queue,
	// update the appropriate end(s) directly.
	if q.head == info {
		q.head = info.next
	}
	if q.tail == info {
		q.tail = info.prev
	}

	// Remove the unit from the list.
	if info.prev != nil {
		info.prev.next = info.next
	}
	if info.next != nil {
		info.next.prev = info.prev
	}
	info.prev = nil
	info.next = nil
}
