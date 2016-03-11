// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package life

import (
	"github.com/juju/errors"
)

// Value indicates the state of some entity.
type Value string

const (
	// Alive indicates that some entity is meant to exist.
	Alive Value = "alive"

	// Dying indicates that some entity should be removed.
	Dying Value = "dying"

	// Dead indicates that some entity is no longer useful,
	// and can be destroyed unconditionally.
	Dead Value = "dead"
)

// Validate returns an error if the value is not known.
func (v Value) Validate() error {
	switch v {
	case Alive, Dying, Dead:
		return nil
	}
	return errors.NotValidf("life value %q", v)
}

// Predicate is a predicate.
type Predicate func(Value) bool

// IsNotAlive is a Predicate that returns true if the supplied value
// is not Alive.
//
// This generally indicates that the entity in question is at some
// stage of destruction/cleanup.
func IsNotAlive(v Value) bool {
	return v != Alive
}

// IsNotDead is a Predicate that returns true if the supplied value
// is not Dead.
//
// This generally indicates that the entity in question is active in
// some way, and can probably not be completely destroyed without
// consequences.
func IsNotDead(v Value) bool {
	return v != Dead
}
