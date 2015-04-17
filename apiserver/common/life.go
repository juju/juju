// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// LifeGetter implements a common Life method for use by various facades.
type LifeGetter struct {
	st         state.EntityFinder
	getCanRead GetAuthFunc
}

// NewLifeGetter returns a new LifeGetter. The GetAuthFunc will be used on
// each invocation of Life to determine current permissions.
func NewLifeGetter(st state.EntityFinder, getCanRead GetAuthFunc) *LifeGetter {
	return &LifeGetter{
		st:         st,
		getCanRead: getCanRead,
	}
}

func (lg *LifeGetter) oneLife(tag names.Tag) (params.Life, error) {
	entity0, err := lg.st.FindEntity(tag)
	if err != nil {
		return "", err
	}
	entity, ok := entity0.(state.Lifer)
	if !ok {
		return "", NotSupportedError(tag, "life cycles")
	}
	return params.Life(entity.Life().String()), nil
}

// Life returns the life status of every supplied entity, where available.
func (lg *LifeGetter) Life(args params.Entities) (params.LifeResults, error) {
	result := params.LifeResults{
		Results: make([]params.LifeResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := lg.getCanRead()
	if err != nil {
		return params.LifeResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		err = ErrPerm
		if canRead(tag) {
			result.Results[i].Life, err = lg.oneLife(tag)
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}
