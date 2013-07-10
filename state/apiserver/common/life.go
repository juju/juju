// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// LifeGetter implements a common Life method for use by various facades.
type LifeGetter struct {
	st         LiferGetter
	getCanRead GetAuthFunc
}

type LiferGetter interface {
	Lifer(tag string) (state.Lifer, error)
}

// NewLifeGetter returns a new LifeGetter. The GetAuthFunc will be used on
// each invocation of Life to determine current permissions.
func NewLifeGetter(st LiferGetter, getCanRead GetAuthFunc) *LifeGetter {
	return &LifeGetter{
		st:         st,
		getCanRead: getCanRead,
	}
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
		return params.LifeResults{}, err
	}
	for i, entity := range args.Entities {
		err := ErrPerm
		if canRead(entity.Tag) {
			var lifer state.Lifer
			lifer, err = lg.st.Lifer(entity.Tag)
			if err == nil {
				result.Results[i].Life = params.Life(lifer.Life().String())
			}
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}
