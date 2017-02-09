// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"bytes"
	"fmt"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/juju/loggo"
)

// GlobalTracker tracks the locations of all the State instances
// created through the various paths. Both ForModel and Open end up
// going through the internal newState method.
var GlobalTracker = newTracker()

type trackerStack struct {
	uuid string
	// Where was this State object created.
	stack string
	// Has the state been closed.
	closed bool
	// When the object was added, and closed
	whenAdded  time.Time
	whenClosed time.Time
}

// Tracker records the locations of all the active *State instances.
type Tracker struct {
	mu         sync.Mutex
	references map[string]*trackerStack
	logger     loggo.Logger
}

func newTracker() *Tracker {
	return &Tracker{
		references: make(map[string]*trackerStack),
		logger:     loggo.GetLogger("juju.state.tracker"),
	}
}

// We always use a uint64 as a key rather than the pointer itself
// as to not hold a reference to the object.
func (t *Tracker) key(s *State) string {
	return fmt.Sprintf("%p", s)
}

// Add records the stack and uses the address of the State object
// as a key to the internal map to record the location that the instance
// was created from.
func (t *Tracker) Add(s *State) {
	stack := debug.Stack()
	t.mu.Lock()
	defer t.mu.Unlock()

	// Use the pointer of the State as a key into the map.
	key := t.key(s)

	item := &trackerStack{
		uuid:      s.ModelUUID(),
		stack:     string(stack),
		whenAdded: time.Now(),
	}
	t.logger.Debugf("[%p] adding key %v for %s", t, key, item.uuid)
	t.references[key] = item

	removeRef := func(st *State) {
		t.mu.Lock()
		defer t.mu.Unlock()
		key := t.key(st)
		delete(t.references, key)
		t.logger.Debugf("[%p] removing key %v for %s", t, key, item.uuid)
	}

	runtime.SetFinalizer(s, removeRef)
}

// RecordClosed marks the State instance as closed for reporting purposes.
func (t *Tracker) RecordClosed(s *State) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := t.key(s)
	if item, found := t.references[key]; found {
		t.logger.Tracef("marking closed key %v for %s", key, item.uuid)
		item.closed = true
		item.whenClosed = time.Now()
	} else {
		t.logger.Debugf("missing state reference for %s", s.ModelUUID())
	}
}

// count is used for testing only.
func (t *Tracker) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.references)
}

// IntrospectionReport is used by the introspection worker to output
// the information for the user.
func (t *Tracker) IntrospectionReport() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	var (
		now    = time.Now().Round(time.Second)
		buff   = &bytes.Buffer{}
		open   []*trackerStack
		closed []*trackerStack
	)

	for _, item := range t.references {
		if item.closed {
			closed = append(closed, item)
		} else {
			open = append(open, item)
		}
	}

	sort.Sort(oldestFirst(open))
	sort.Sort(oldestFirst(closed))

	outputItem := func(item *trackerStack) {
		fmt.Fprintf(buff, "\nModel: %s\n", item.uuid)
		d := now.Sub(item.whenAdded.Round(time.Second))
		fmt.Fprintf(buff, "  Added: %s (%s ago)\n", item.whenAdded.Format(time.RFC1123), d)
		if item.closed {
			d := item.whenClosed.Sub(item.whenAdded)
			fmt.Fprintf(buff, "  Closed: %s (open for %s)\n", item.whenClosed.Format(time.RFC1123), d)
		}
		fmt.Fprintf(buff, "  Location: \n%s\n", item.stack)
	}

	for _, item := range open {
		outputItem(item)
	}
	for _, item := range closed {
		outputItem(item)
	}

	return fmt.Sprintf(""+
		"Total count: %d\n"+
		"Closed count: %d\n"+
		"\n%s", len(t.references), len(closed), buff)
}

type oldestFirst []*trackerStack

func (d oldestFirst) Len() int           { return len(d) }
func (d oldestFirst) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d oldestFirst) Less(i, j int) bool { return d[i].whenAdded.Before(d[j].whenAdded) }
