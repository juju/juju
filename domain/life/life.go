// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package life

import (
	corelife "github.com/juju/juju/core/life"
)

// Life represents the life of an entity
// as recorded in the life lookup table.
type Life int

const (
	Alive Life = iota
	Dying
	Dead
)

// ToCoreLife converts a life value to a core life value.
func (l *Life) ToCoreLife() corelife.Value {
	var lifeValue corelife.Value
	switch *l {
	case Alive:
		lifeValue = corelife.Alive
	case Dying:
		lifeValue = corelife.Dying
	case Dead:
		lifeValue = corelife.Dead
	}
	return lifeValue
}

// FromCoreLife converts a core life value into a domain life value.
func FromCoreLife(l corelife.Value) Life {
	var lifeValue Life
	switch l {
	case corelife.Alive:
		lifeValue = Alive
	case corelife.Dying:
		lifeValue = Dying
	case corelife.Dead:
		lifeValue = Dead
	}
	return lifeValue
}
