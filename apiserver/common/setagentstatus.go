// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

// Agentstatussetter implements a SetAgentStatus method to be
// used by facades that discern between unit and agent for setting status
type AgentStatusSetter struct {
	StatusSetter
}

// NewAgentStatusSetter returns a new AgentStatusSetter. The GetAuthFunc will be
// used on each invocation of SetStatus to determine current
// permissions.
func NewAgentStatusSetter(st state.EntityFinder, getCanModify GetAuthFunc) *AgentStatusSetter {
	statusSetter := &AgentStatusSetter{
		StatusSetter{
			st:           st,
			getCanModify: getCanModify,
		},
	}
	setterFunc := func() SetterFunc { return statusSetter.setStatus }
	statusSetter.getSetterFunc = setterFunc
	return statusSetter
}

func (s *AgentStatusSetter) setUnitAgentStatus(tag names.UnitTag, status params.Status, info string, data map[string]interface{}) error {
	entity, err := s.st.FindEntity(tag)
	if err != nil {
		return errors.Trace(err)
	}
	switch unit := entity.(type) {
	case state.AgentUnit:
		agent := unit.Agent()
		return agent.SetStatus(state.Status(status), info, data)
	default:
		return errors.Errorf("cannot set agent status for entity %q", entity)
	}
}

func (s *AgentStatusSetter) setStatus(tag names.Tag, status params.Status, info string, data map[string]interface{}) error {
	unitTag := tag.(names.UnitTag)
	return s.setUnitAgentStatus(unitTag, status, info, data)
}
