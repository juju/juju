// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

// DeadEnsurer implements a common EnsureDead method for use by
// various facades.
type DeadEnsurer struct {
	st           state.EntityFinder
	getCanModify GetAuthFunc
}

// NewDeadEnsurer returns a new DeadEnsurer. The GetAuthFunc will be
// used on each invocation of EnsureDead to determine current
// permissions.
func NewDeadEnsurer(st state.EntityFinder, getCanModify GetAuthFunc) *DeadEnsurer {
	return &DeadEnsurer{
		st:           st,
		getCanModify: getCanModify,
	}
}

func (d *DeadEnsurer) ensureEntityDead(tag names.Tag) error {
	entity0, err := d.st.FindEntity(tag)
	if err != nil {
		return err
	}
	entity, ok := entity0.(state.EnsureDeader)
	if !ok {
		return NotSupportedError(tag, "ensuring death")
	}
	return entity.EnsureDead()
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
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			return params.ErrorResults{}, errors.Trace(err)
		}

		err = ErrPerm
		if canModify(tag) {
			err = d.ensureEntityDead(tag)
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}
