// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

// Unit represents the state of a service
// and its subordinate parts.
type Unit struct {
	topology *topology
	id       string
}

// Id returns the unit id.
func (u Unit) Id() string {
	return u.id
}
