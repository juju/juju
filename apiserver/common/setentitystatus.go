// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

type EntityStatusSetter struct {
	StatusSetter
}

// NewEntityStatusSetter returns a new StatusSetter. The GetAuthFunc will be
// used on each invocation of SetStatus to determine current
// permissions.
func NewEntityStatusSetter(st state.EntityFinder, getCanModify GetAuthFunc) *EntityStatusSetter {
	statusSetter := &EntityStatusSetter{
		StatusSetter{
			st:           st,
			getCanModify: getCanModify,
		},
	}
	setterFunc := func() SetterFunc { return statusSetter.setStatus }
	statusSetter.getSetterFunc = setterFunc
	return statusSetter
}

func (s *EntityStatusSetter) setEntityStatus(tag names.Tag, status params.Status, info string, data map[string]interface{}) error {
	entity, err := s.st.FindEntity(tag)
	if err != nil {
		return err
	}
	switch entity := entity.(type) {
	case state.AgentUnit:
		agent := entity.Agent()
		return agent.SetStatus(state.Status(status), info, data)
	case state.StatusSetter:
		return entity.SetStatus(state.Status(status), info, data)
	default:
		return NotSupportedError(tag, fmt.Sprintf("setting status, %T", entity))
	}
}

func (s *EntityStatusSetter) setStatus(tag names.Tag, status params.Status, info string, data map[string]interface{}) error {
	return s.setEntityStatus(tag, status, info, data)
}
