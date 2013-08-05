// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// StatusSetter implements a common SetStatus method for use by
// various facades.
type StatusSetter struct {
	st           StatusSetterer
	getCanModify GetAuthFunc
}

type StatusSetterer interface {
	// state.State implements StatusSetter to provide wats for us to
	// call object.SetStatus (for machines, units, etc). This is used
	// to allow us to test with mocks without having to actually bring
	// up state.
	StatusSetter(tag string) (state.StatusSetter, error)
}

// NewStatusSetter returns a new StatusSetter. The GetAuthFunc will be
// used on each invocation of SetStatus to determine current
// permissions.
func NewStatusSetter(st StatusSetterer, getCanModify GetAuthFunc) *StatusSetter {
	return &StatusSetter{
		st:           st,
		getCanModify: getCanModify,
	}
}

func (s *StatusSetter) setEntityStatus(tag string, status params.Status, info string) error {
	statusSetter, err := s.st.StatusSetter(tag)
	if err != nil {
		return err
	}
	return statusSetter.SetStatus(status, info)
}

// SetStatus sets the status of each given entity.
func (s *StatusSetter) SetStatus(args params.SetStatus) (params.ErrorResults, error) {
	// This is only to ensure compatibility with v1.12.
	// DEPRECATE(v1.14)
	if len(args.Entities) == 0 {
		args.Entities = args.Machines
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := s.getCanModify()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.Entities {
		err := ErrPerm
		if canModify(arg.Tag) {
			err = s.setEntityStatus(arg.Tag, arg.Status, arg.Info)
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}
