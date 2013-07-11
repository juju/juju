// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// Remover implements a common Remove method for use by various facades.
type Remover struct {
	st           Removerer
	getCanModify GetAuthFunc
}

type Removerer interface {
	// state.State implements Remover to provide ways for us to call
	// object.Remove (for Units, etc). This is used to allow us to
	// test with mocks without having to actually bring up state.
	Remover(tag string) (state.Remover, error)
}

// NewRemover returns a new Remover. The GetAuthFunc will be used on
// each invocation of Remove to determine current permissions.
func NewRemover(st Removerer, getCanModify GetAuthFunc) *Remover {
	return &Remover{
		st:           st,
		getCanModify: getCanModify,
	}
}

func (r *Remover) removeEntity(entity params.Entity) (err error) {
	var remover state.Remover
	if remover, err = r.st.Remover(entity.Tag); err != nil {
		return err
	}
	if err = remover.EnsureDead(); err != nil {
		return err
	}
	return remover.Remove()
}

// Remove removes every given entity from state, calling EnsureDead
// first, then Remove. It will fail if the entity is not present.
func (r *Remover) Remove(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Errors: make([]*params.Error, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := r.getCanModify()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		err := ErrPerm
		if canModify(entity.Tag) {
			err = r.removeEntity(entity)
		}
		result.Errors[i] = ServerError(err)
	}
	return result, nil
}
