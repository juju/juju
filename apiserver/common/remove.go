// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// Remover implements a common Remove method for use by various facades.
type Remover struct {
	st             state.EntityFinder
	store          objectstore.ObjectStore
	afterDead      func(tag names.Tag)
	callEnsureDead bool
	getCanModify   GetAuthFunc
}

// NewRemover returns a new Remover. The callEnsureDead flag specifies
// whether EnsureDead should be called on an entity before
// removing. The GetAuthFunc will be used on each invocation of Remove
// to determine current permissions.
func NewRemover(st state.EntityFinder, store objectstore.ObjectStore, afterDead func(tag names.Tag), callEnsureDead bool, getCanModify GetAuthFunc) *Remover {
	return &Remover{
		st:             st,
		store:          store,
		afterDead:      afterDead,
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
		return apiservererrors.NotSupportedError(tag, "removal")
	}
	// Only remove entities that are not Alive.
	if life := remover.Life(); life == state.Alive {
		return fmt.Errorf("cannot remove entity %q: still alive", tag.String())
	}
	if r.callEnsureDead {
		if err := remover.EnsureDead(); err != nil {
			return err
		}
		if r.afterDead != nil {
			r.afterDead(tag)
		}
	}
	// TODO (anastasiamac) this needs to work with force if needed
	return remover.Remove(r.store)
}

// Remove removes every given entity from state, calling EnsureDead
// first, then Remove. It will fail if the entity is not present.
func (r *Remover) Remove(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
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
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canModify(tag) {
			err = r.removeEntity(tag)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// MaxWait is how far in the future the backstop force cleanup will be scheduled.
// Default is 1min if no value is provided.
func MaxWait(in *time.Duration) time.Duration {
	if in != nil {
		return *in
	}
	return 1 * time.Minute
}
