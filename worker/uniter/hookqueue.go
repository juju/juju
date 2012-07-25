package uniter

import (
	"launchpad.net/juju-core/state"
	"sort"
)

// HookInfo holds details required to execute a relation hook.
type HookInfo struct {
	HookKind   string
	RemoteUnit string
	Version    int
	// Members contains a map[string]interface{} for every remote unit,
	// holding its relation settings, keyed on unit name.
	Members map[string]map[string]interface{}
}

// HookQueue starts a goroutine which receives state.RelationUnitsChange
// events and sends corresponding HookInfo values. When the input channel
// is closed, it will terminate immediately.
func HookQueue(out chan<- HookInfo, in <-chan state.RelationUnitsChange) {
	q := &hookQueue{
		in:   in,
		out:  out,
		info: map[string]*unitInfo{},
	}
	go q.loop()
}

// hookQueue aggregates RelationUnitsChanged events and ensures that
// the HookInfo values it sends always reflect the latest known state
// of the relation.
type hookQueue struct {
	in  <-chan state.RelationUnitsChange
	out chan<- HookInfo

	// info holds information about all units that were added to the
	// queue and haven't had a "departed" event popped. This means the
	// unit may be in info and not currently in the queue itself.
	info map[string]*unitInfo

	// head and tail are the ends of the queue.
	head, tail *unitInfo

	// joined, if not empty, indicates that the most recent popped event
	// was a "joined" for the named unit (and therefore that next must
	// point to a "changed" for that same unit). If joined is not empty,
	// the queue is considered non-empty, even if head is nil.
	joined string

	// next holds a HookInfo corresponding to the head of the queue.
	// It is only valid if the queue is not empty.
	next HookInfo
}

// unitInfo holds unit information for management by hookQueue.
type unitInfo struct {
	// unit holds the name of the unit.
	unit string
	// version and settings hold the most recent settings known
	// to the hookQueue.
	version  int
	settings map[string]interface{}
	// present is true if the unit was a member of the relation
	// after the most recent event was popped. In practice, it
	// is false until a "joined" is popped for this unit and
	// true for the remaining lifetime of the unitInfo.
	present bool
	// hook holds the current idea of the next hook that should
	// be run for the unit, and is empty if and only if the unit
	// is not queued.
	hook string
	// prev and next define the position in the queue of the
	// unit's next hook.
	prev, next *unitInfo
}

func (q *hookQueue) loop() {
	var out chan<- HookInfo
	for {
		if q.head == nil && q.joined == "" {
			// The queue is empty; q.next is invalid; ensure we don't send it.
			out = nil
		} else {
			out = q.out
		}
		select {
		case ch, ok := <-q.in:
			if !ok {
				return
			}
			q.update(ch)
		case out <- q.next:
			q.pop()
		}
	}
}

// update modifies the queue such that the HookInfo values it sends will
// reflect the supplied change.
func (q *hookQueue) update(ruc state.RelationUnitsChange) {
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
			// If known, and not already in queue, and the settings
			// version really has changed, queue a change.
			if settings.Version != info.version {
				q.add(unit, "changed")
			}
		} else if info.hook == "departed" {
			// If settings have changed, queue a change; otherwise
			// just elide the depart.
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
		if hook := q.info[unit].hook; hook == "joined" {
			q.remove(unit)
		} else {
			q.add(unit, "departed")
		}
	}
	q.setNext()
}

// pop advances the queue. It will panic if the queue is already empty.
func (q *hookQueue) pop() {
	if q.joined != "" {
		if q.info[q.joined].hook == "changed" {
			// We just ran this very hook; no sense keeping it queued.
			q.remove(q.joined)
		}
		q.joined = ""
	} else {
		if q.head == nil {
			panic("queue is empty")
		}
		old := *q.head
		q.remove(q.head.unit)
		if old.hook == "joined" {
			q.joined = old.unit
			q.info[old.unit].present = true
		} else if old.hook == "departed" {
			delete(q.info, old.unit)
		}
	}
	q.setNext()
}

// setNext sets q.next such that it reflects the current state of the queue.
func (q *hookQueue) setNext() {
	var unit, hook string
	if q.joined != "" {
		unit = q.joined
		hook = "changed"
	} else {
		if q.head == nil {
			// The queue is empty; just leave the stale HookInfo around,
			// because we won't send it anyway.
			return
		}
		unit = q.head.unit
		hook = q.head.hook
	}
	version := q.info[unit].version
	members := make(map[string]map[string]interface{})
	for unit, info := range q.info {
		if info.present {
			members[unit] = info.settings
		}
	}
	if hook == "joined" {
		members[unit] = q.info[unit].settings
	} else if hook == "departed" {
		delete(members, unit)
	}
	q.next = HookInfo{hook, unit, version, members}
}

// add sets the next hook to be run for the named unit, and places it
// at the tail of the queue if it is not already queued. It will panic
// if the unit is not in q.info.
func (q *hookQueue) add(unit, hook string) {
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
func (q *hookQueue) remove(unit string) {
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
