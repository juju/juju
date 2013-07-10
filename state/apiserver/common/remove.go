// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// Remover implements a common Remove method for use by various facades.
type Remover struct {
	st         RemoverGetter
	getCanRead GetAuthFunc
}

type RemoverGetter interface {
	Remover(tag string) (state.Remover, error)
}

// NewRemover returns a new Remover. The GetAuthFunc will be used on
// each invocation of Remove to determine current permissions.
func NewRemover(st RemoverGetter, getCanRead GetAuthFunc) *Remover {
	return &Remover{
		st:         st,
		getCanRead: getCanRead,
	}
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
	canRead, err := r.getCanRead()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		err := ErrPerm
		if canRead(entity.Tag) {
			var remover state.Remover
			remover, err = r.st.Remover(entity.Tag)
			if err == nil {
				if err = remover.EnsureDead(); err == nil {
					err = remover.Remove()
				}
			}
		}
		result.Errors[i] = ServerError(err)
	}
	return result, nil
}
