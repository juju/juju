// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// DeadEnsurer implements a common EnsureDead method for use by
// various facades.
type DeadEnsurer struct {
	st           DeadEnsurerer
	getCanModify GetAuthFunc
}

type DeadEnsurerer interface {
	// state.State implements DeadEnsurer to provide ways for us to
	// call object.EnsureDead (for machines, units, etc). This is used
	// to allow us to test with mocks without having to actually bring
	// up state.
	DeadEnsurer(tag string) (state.DeadEnsurer, error)
}

// NewDeadEnsurer returns a new DeadEnsurer. The GetAuthFunc will be
// used on each invocation of EnsureDead to determine current
// permissions.
func NewDeadEnsurer(st DeadEnsurerer, getCanModify GetAuthFunc) *DeadEnsurer {
	return &DeadEnsurer{
		st:           st,
		getCanModify: getCanModify,
	}
}

func (d *DeadEnsurer) ensureEntityDead(tag string) error {
	deadEnsurer, err := d.st.DeadEnsurer(tag)
	if err != nil {
		return err
	}
	return deadEnsurer.EnsureDead()
}

// EnsureDead calls EnsureDead on each given entity from state. It
// will fail if the entity is not present. If it's Alive, nothing will
// happen (see state/EnsureDead() for units or machines).
func (d *DeadEnsurer) EnsureDead(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := d.getCanModify()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		err := ErrPerm
		if canModify(entity.Tag) {
			err = d.ensureEntityDead(entity.Tag)
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}
