// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

// Remover implements a common Remove method for use by various facades.
type Remover struct {
	st             state.EntityFinder
	callEnsureDead bool
	getCanModify   GetAuthFunc
}

// NewRemover returns a new Remover. The callEnsureDead flag specifies
// whether EnsureDead should be called on an entity before
// removing. The GetAuthFunc will be used on each invocation of Remove
// to determine current permissions.
func NewRemover(st state.EntityFinder, callEnsureDead bool, getCanModify GetAuthFunc) *Remover {
	return &Remover{
		st:             st,
		callEnsureDead: callEnsureDead,
		getCanModify:   getCanModify,
	}
}

func (r *Remover) removeEntity(tag names.Tag) error {
	entity, err := r.st.FindEntity(tag)
	if err != nil {
		return err
	}
	remover, ok := entity.(interface {
		state.Lifer
		state.Remover
		state.EnsureDeader
	})
	if !ok {
		return NotSupportedError(tag, "removal")
	}
	// Only remove entites that are not Alive.
	if life := remover.Life(); life == state.Alive {
		return fmt.Errorf("cannot remove entity %q: still alive", tag.String())
	}
	if r.callEnsureDead {
		if err := remover.EnsureDead(); err != nil {
			return err
		}
	}
	return remover.Remove()
}

// Remove removes every given entity from state, calling EnsureDead
// first, then Remove. It will fail if the entity is not present.
func (r *Remover) Remove(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := r.getCanModify()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		err = ErrPerm
		if canModify(tag) {
			err = r.removeEntity(tag)
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}
