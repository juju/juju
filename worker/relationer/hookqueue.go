package relationer

import (
	"fmt"
	"launchpad.net/juju-core/state"
	"sort"
)

// HookInfo holds everything we need to know to execute a relation hook,
// assuming we have access to the relation itself and the unit executing
// the hook.
type HookInfo struct {
	Kind    string
	Unit    string
	Members map[string]map[string]interface{}
}

// HookQueue accepts state.RelationUnitsChange events and converts them
// into HookInfos.
type HookQueue struct {

	// members holds the names of the relation units known to the last
	// HookInfo emitted from the queue; from the point of view of the
	// unit agent, it is the "present" membership of the relation.
	members map[string]struct{}

	// settings holds the latest settings and version for every relation
	// unit known to the queue. This includes those for relation units
	// from the "future": ie those which are in the queue, but have not
	// yet been involved in any hook execution.
	settings map[string]state.UnitSettings

	// changes is an ordered list of hooks we expect to execute in the
	// future. Not every change is meaningful: when a newer event
	// renders an earlier one obsolete, we clear out the obsolete change's
	// hook and henceforth ignore it (rather than compacting the list).
	// changes must not contain more than one valid entry per unit.
	changes []change

	// inflight holds the most recent HookInfo returned from Next(), until
	// Done is called, at which point it will be cleared.
	inflight *HookInfo
}

// change is used internally by HookQueue.
type change struct {
	hook string
	unit string
}

// NewHookQueue returns an empty HookQueue.
func NewHookQueue() *HookQueue {
	return &HookQueue{
		members:  map[string]struct{}{},
		settings: map[string]state.UnitSettings{},
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
		if oldSettings, found := q.settings[unit]; !found {
			// If not currently known, queue a join.
			q.changes = append(q.changes, change{"joined", unit})
		} else if idx := q.changeIndex(unit); idx == -1 {
			// If known, and not already in queue, queue a change.
			q.changes = append(q.changes, change{"changed", unit})
		} else if ch := q.changes[idx]; ch.hook == "departed" {
			// If known, and queued for depart, clear out the old entry...
			q.changes[idx].hook = ""
			if settings.Version != oldSettings.Version {
				// ...and queue a change if justified.
				q.changes = append(q.changes, change{"changed", unit})
			}
		} // Otherwise, it's already queued for either join or change; ignore.

		// Always update the stored settings.
		q.settings[unit] = settings
	}

	for _, unit := range ruc.Departed {
		if idx := q.changeIndex(unit); idx == -1 {
			// If not present in queue, queue for depart.
			q.changes = append(q.changes, change{"departed", unit})
		} else {
			// Otherwise, clear out the old entry...
			hook := q.changes[idx].hook
			q.changes[idx].hook = ""
			if hook == "changed" {
				// ...and schedule a deletion if justified.
				q.changes = append(q.changes, change{"departed", unit})
			}
		}
	}
}

// Next returns information about the next hook to fire. If the returned
// bool is false, the HookInfo is not valid and the queue is currently empty.
// If the HookInfo is valid, subsequent calls to Next will return the same
// HookInfo, until Done is called.
func (q *HookQueue) Next() (HookInfo, bool) {
	if q.inflight != nil {
		// We're retrying a hook that previously failed to execute.
		return *q.inflight, true
	}

	idx := q.nextChangeIndex()
	if idx == -1 {
		// The queue is empty.
		return HookInfo{}, false
	}

	// Update queue state.
	ch := q.changes[idx]
	prefix := []change{}
	if ch.hook == "joined" {
		// Insert a changed event at the head of the queue.
		q.members[ch.unit] = struct{}{}
		prefix = append(prefix, change{"changed", ch.unit})
	} else if ch.hook == "departed" {
		// Because we only have one queued event per unit, we can
		// forget all about this unit for now.
		delete(q.members, ch.unit)
		delete(q.settings, ch.unit)
	}
	q.changes = append(prefix, q.changes[idx+1:]...)

	// Create the HookInfo, using the *latest* relation unit settings,
	// and place it in inflight for potential reuse if the hook fails.
	result := HookInfo{ch.hook, ch.unit, make(map[string]map[string]interface{})}
	for m := range q.members {
		result.Members[m] = q.settings[m].Settings
	}
	q.inflight = &result
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

// changeIndex returns the index of the queued change for the supplied unit
// name, or -1 if no changes are queued for that unit.
func (q *HookQueue) changeIndex(unit string) int {
	for idx, ch := range q.changes {
		if ch.hook != "" && ch.unit == unit {
			return idx
		}
	}
	return -1
}

// nextChangeIndex returns the index of the first valid change in the queue,
// or -1 if the queue is empty.
func (q *HookQueue) nextChangeIndex() int {
	for idx, ch := range q.changes {
		if ch.hook != "" {
			return idx
		}
	}
	return -1
}

func remove(old []string, target string) (new []string) {
	ok := false
	for _, v := range old {
		if v == target {
			ok = true
			continue
		}
		new = append(new, v)
	}
	if !ok {
		panic(fmt.Errorf("%q not present in %#v", target, old))
	}
	return new
}
