// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"sync"
)

// Unit represents the state of a service
// and its subordinate parts.
type Unit struct {
	writeLock sync.Mutex
	topology  *topology
	id        string
	Exposed   bool "exposed"
	Sequence  int  "sequence"
}

// Id returns the unit id.
func (u Unit) Id() string {
	return u.id
}

// sync synchronizes the unit after an update event. This
// is done recurively with all entities below.
func (u *Unit) sync(newUnit *Unit) error {
	u.writeLock.Lock()
	defer u.writeLock.Unlock()

	// 1. Own fields.
	u.Exposed = newUnit.Exposed
	u.Sequence = newUnit.Sequence

	return nil
}
