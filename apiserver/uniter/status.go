// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"

	"github.com/juju/juju/state"
)

type StatusAPI struct {
	*common.StatusSetter
	st           state.EntityFinder
	getCanModify common.GetAuthFunc
}

// NewStatusAPI creates a new server-side Status setter API facade.
func NewStatusAPI(st state.EntityFinder, getCanModify common.GetAuthFunc) *StatusAPI {
	setter := common.NewStatusSetter(st, getCanModify)
	return &StatusAPI{
		StatusSetter: setter,
		st:           st,
		getCanModify: getCanModify,
	}
}

// SetStatus will set status for a entities passed in args. If the entity is
// a Unit it will instead set status to its agent, to emulate backwards
// compatibility.
func (s *StatusAPI) SetStatus(args params.SetStatus) (params.ErrorResults, error) {
	setter := NewEntityStatusSetter(s.st, s.getCanModify)
	return setter.SetStatus(args)
}

// SetAgentStatus will set status for agents of Units passed in args, if one
// of the args is not an Unit it will fail.
func (s *StatusAPI) SetAgentStatus(args params.SetStatus) (params.ErrorResults, error) {
	setter := NewAgentStatusSetter(s.getCanModify)
	return setter.SetStatus(args)
}

// SetUnitStatus sets status for all elements passed in args, the difference
// with SetStatus is that if an entity is a Unit it will set its status instead
// of its agent.
func (s *StatusAPI) SetUnitStatus(args params.SetStatus) (params.ErrorResults, error) {
	return s.StatusSetter.SetStatus(args)
}
