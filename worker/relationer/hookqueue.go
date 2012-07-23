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

// HookQueue accepts state.RelationUnitsChange events and converts them
// into HookInfos.
type HookQueue struct {
	// head and tail are the ends of the queue.
	head *unitInfo
	tail *unitInfo

	// current and members track the present state of the relation, from the
	// perspective of the client. They are used to create HookInfos to return
	// from Prev and Next; Next always updates the current unit and membership,
	// and Prev always uses the values set by the previous call to Next.
	current *unitInfo
	members map[string]struct{}

	// info holds the unitInfo for every known remote unit; it should hold
	// an entry for every unit in members, and one for every unit queued
	// for joining. It is used, along with current and members, to help
	// create HookInfo, by providing their latest settings data.
	info map[string]*unitInfo
}

// unitInfo is used internally by HookQueue. It holds information about a
// unit that is, or has been, or will be, a member of the relation; and
// if appropriate, what hook should be run next for that unit, and when.
type unitInfo struct {
	unit     string
	version  int
	settings map[string]interface{}
	hook     string
	prev     *unitInfo
	next     *unitInfo
}

// NewHookQueue returns an empty HookQueue.
func NewHookQueue() *HookQueue {
	return &HookQueue{
		members: map[string]struct{}{},
		info:    map[string]*unitInfo{},
	}
}

// Add updates and compacts the queue, ensuring that there is no more than
// one queued hook per remote unit.
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

// Ready returns true if there is a hook ready to run.
func (q *HookQueue) Ready() bool {
	return q.head != nil
}

// Next returns a HookInfo describing the next hook to run. If no hook
// is ready to run, Next will panic.
func (q *HookQueue) Next() HookInfo {
	if q.head == nil {
		panic("queue is empty")
	}
	info := *q.head
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
	q.current = &info
	return q.hookInfo()
}

// Prev returns a new HookInfo configured as the one previously
// returned from Next, but with updated settings in Members. If
// there is no such hook available, Prev will panic.
func (q *HookQueue) Prev() HookInfo {
	if prev := q.current; prev == nil {
		panic("no previously returned hook")
	} else if prev.hook == "changed" {
		// Hey, we may be able to remove a redundant event from the queue!
		next := q.info[prev.unit]
		if prev.version != next.version && next.hook == "changed" {
			q.remove(next.unit)
		}
	}
	return q.hookInfo()
}

// hookInfo creates and returns a HookInfo corresponding to q.current.
func (q *HookQueue) hookInfo() HookInfo {
	hi := HookInfo{
		HookName:   q.current.hook,
		RemoteUnit: q.current.unit,
		Members:    make(map[string]map[string]interface{}),
	}
	for m := range q.members {
		hi.Members[m] = q.info[m].settings
	}
	return hi
}

// append places the named unit at the tail of the queue. It will
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

// remove removes the named unit from the queue, if it is present.
// It will panic if the unit is not known.
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
