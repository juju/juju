// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package life

import (
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/errors"
)

// Life represents the life of an entity
// as recorded in the life lookup table.
type Life int

const (
	Alive Life = iota
	Dying
	Dead
)

// Value returns the [github.com/juju/juju/core/life.Life]
// value corresponding to this life.
func (l Life) Value() (corelife.Value, error) {
	switch l {
	case Alive:
		return corelife.Alive, nil
	case Dying:
		return corelife.Dying, nil
	case Dead:
		return corelife.Dead, nil
	}
	return "", errors.Errorf("invalid life value %d", l)
}
