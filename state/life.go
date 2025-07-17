// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/life"
)

// Life represents the lifecycle state of the entities
// Relation, Unit, Application and Machine.
type Life int8

const (
	Alive Life = iota
	Dying
	Dead
)

// String is deprecated, use Value.
func (l Life) String() string {
	switch l {
	case Alive:
		return "alive"
	case Dying:
		return "dying"
	case Dead:
		return "dead"
	default:
		return "alive"
	}
}

// Value returns the core.life.Value type.
func (l Life) Value() life.Value {
	switch l {
	case Alive:
		return life.Alive
	case Dying:
		return life.Dying
	case Dead:
		return life.Dead
	default:
		return life.Alive
	}
}
