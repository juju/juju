package uniter

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
	"sort"
)

// hookQueue is the minimal interface implemented by both HookQueue and
// BrokenHookQueue.
type hookQueue interface {
	hookQueue()
	Stop() error
}

// RelationUnitsWatcher is used to enable deterministic testing, by
// supplying a reliable stream of RelationUnitsChange events; usually,
// it will be a *state.RelationUnitsWatcher.
type RelationUnitsWatcher interface {
	Err() error
	Stop() error
	Changes() <-chan state.RelationUnitsChange
}

// HookInfo holds details required to execute a relation hook.
type HookInfo struct {
	// RelationId identifies the relation associated with the hook queue.
	RelationId int

	// HookKind is one of "joined", "changed", "departed", or "broken".
	HookKind string

	// RemoteUnit is the unit name associated with HookKind.
	RemoteUnit string

	// ChangeVersion identifies the most recent unit settings change
	// associated with RemoteUnit.
	ChangeVersion int

	// Members is a map from remote unit name to its settings.
	// It contains all members known in the relation up to the
	// moment in which the HookInfo was delivered.
	Members map[string]map[string]interface{}
}

// HookQueue aggregates values obtained from a relation settings watcher
// and sends out details about hooks that must be executed in the unit.
type HookQueue struct {
	tomb       tomb.Tomb
	w          RelationUnitsWatcher
	out        chan<- HookInfo
	relationId int

	// info holds information about all units that were added to the
	// queue and haven't had a "departed" event popped. This means the
	// unit may be in info and not currently in the queue itself.
	info map[string]*unitInfo

	// head and tail are the ends of the queue.
	head, tail *unitInfo

	// changedPending, if not empty, indicates that the most recently
	// popped event was a "joined" for the named unit, and therefore
	// that the next event must be to a "changed" for that same unit.
	// If changedPending is not empty, the queue is considered non-
	// empty, even if head is nil.
	changedPending string
}

// unitInfo holds unit information for management by HookQueue.
type unitInfo struct {
	// unit holds the name of the unit.
	unit string

	// version and settings hold the most recent settings known
	// to the HookQueue.
	version  int
	settings map[string]interface{}

	// joined is set to true when a "joined" is popped for this unit.
	joined bool

	// hook holds the current idea of the next hook that should
	// be run for the unit, and is empty if and only if the unit
	// is not queued.
	hook string

	// prev and next define the position in the queue of the
	// unit's next hook.
	prev, next *unitInfo
}

// NewHookQueue returns a new HookQueue that aggregates the values obtained
// from the w watcher and sends into out the details about hooks that must
// be executed in the unit. If any values have previously been received from
// w's Changes channel, the HookQueue's behaviour is undefined.
func NewHookQueue(initial *RelationState, out chan<- HookInfo, w RelationUnitsWatcher) *HookQueue {
	q := &HookQueue{
		w:          w,
		out:        out,
		relationId: initial.RelationId,
		info:       map[string]*unitInfo{},
	}
	go q.loop(initial)
	return q
}

func (q *HookQueue) loop(initial *RelationState) {
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
		panic("HookQueue must be started with a fresh RelationUnitsWatcher")
	}
	q.changedPending = initial.ChangedPending
	ch0 := state.RelationUnitsChange{}
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

	var next HookInfo
	var out chan<- HookInfo
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

func (q *HookQueue) hookQueue() {
	panic("interface sentinel method, do not call")
}

// Stop stops the HookQueue and returns any errors encountered during operation
// or while shutting down.
func (q *HookQueue) Stop() error {
	q.tomb.Kill(nil)
	return q.tomb.Wait()
}

// empty returns true if the queue is empty.
func (q *HookQueue) empty() bool {
	return q.head == nil && q.changedPending == ""
}

// update modifies the queue such that the HookInfo values it sends will
// reflect the supplied change.
func (q *HookQueue) update(ruc state.RelationUnitsChange) {
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
			info = &unitInfo{unit: unit}
			q.info[unit] = info
			q.queue(unit, "joined")
		} else if info.hook != "joined" {
			if settings.Version != info.version {
				q.queue(unit, "changed")
			} else {
				q.unqueue(unit)
			}
		}
		info.version = settings.Version
		info.settings = settings.Settings
	}

	for _, unit := range ruc.Departed {
		if hook := q.info[unit].hook; hook == "joined" {
			q.unqueue(unit)
		} else {
			q.queue(unit, "departed")
		}
	}
}

// pop advances the queue. It will panic if the queue is already empty.
func (q *HookQueue) pop() {
	if q.empty() {
		panic("queue is empty")
	}
	if q.changedPending != "" {
		if q.info[q.changedPending].hook == "changed" {
			// We just ran this very hook; no sense keeping it queued.
			q.unqueue(q.changedPending)
		}
		q.changedPending = ""
	} else {
		old := *q.head
		q.unqueue(q.head.unit)
		if old.hook == "joined" {
			q.changedPending = old.unit
			q.info[old.unit].joined = true
		} else if old.hook == "departed" {
			delete(q.info, old.unit)
		}
	}
}

// next returns the next HookInfo value to send.
func (q *HookQueue) next() HookInfo {
	if q.empty() {
		panic("queue is empty")
	}
	var unit, hook string
	if q.changedPending != "" {
		unit = q.changedPending
		hook = "changed"
	} else {
		unit = q.head.unit
		hook = q.head.hook
	}
	version := q.info[unit].version
	members := make(map[string]map[string]interface{})
	for unit, info := range q.info {
		if info.joined {
			members[unit] = info.settings
		}
	}
	if hook == "joined" {
		members[unit] = q.info[unit].settings
	} else if hook == "departed" {
		delete(members, unit)
	}
	return HookInfo{q.relationId, hook, unit, version, members}
}

// queue sets the next hook to be run for the named unit, and places it
// at the tail of the queue if it is not already queued. It will panic
// if the unit is not in q.info.
func (q *HookQueue) queue(unit, hook string) {
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

// unqueue removes the named unit from the queue. It is fine to
// unqueue a unit that is not in the queue, but it will panic if
// the unit is not in q.info.
func (q *HookQueue) unqueue(unit string) {
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

// BrokenHookQueue acts similarly to HookQueue, but its only concern is sending
// a -departed hook for each current unit, and finishing with a -broken hook.
type BrokenHookQueue struct {
	tomb       *tomb.Tomb
	out        chan<- HookInfo
	relationId int
	members    map[string]int
}

// NewBrokenHookQueue returns a new HookQueue that sends out details about the
// hooks that must be executed in a relation that is going away.
func NewBrokenHookQueue(initial *RelationState, out chan<- HookInfo) *BrokenHookQueue {
	q := &BrokenHookQueue{
		out:        out,
		relationId: initial.RelationId,
		members:    map[string]int{},
	}
	for m, v := range initial.Members {
		q.members[m] = v
	}
	go q.loop()
	return q
}

func (q *BrokenHookQueue) loop() {
	for {
		hi := HookInfo{
			RelationId: q.relationId,
			HookKind:   "broken",
		}
		for m, v := range q.members {
			hi.HookKind = "departed"
			hi.RemoteUnit = m
			hi.ChangeVersion = v
			delete(q.members, m)
			break
		}
		select {
		case <-q.tomb.Dying():
			return
		case q.out <- hi:
		}
		if hi.HookKind == "broken" {
			q.tomb.Kill(nil)
			return
		}
	}
}

func (q *BrokenHookQueue) hookQueue() {
	panic("interface sentinel method, do not call")
}

// Stop stops the BrokenHookQueue and returns any errors encountered
// during operation or while shutting down.
func (q *BrokenHookQueue) Stop() error {
	q.tomb.Kill(nil)
	return q.tomb.Wait()
}
