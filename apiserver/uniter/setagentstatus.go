// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

// Agentstatussetter implements a SetAgentStatus method to be
// used by facades that discern between unit and agent for setting status
type AgentStatusSetter struct {
	*common.StatusSetter
}

type AgentEntityFinder struct {
	st state.EntityFinder
}

func (a *AgentEntityFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	_, ok := tag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("unsupported tag %T", tag)
	}

	entity, err := a.st.FindEntity(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	unit, err := toUnit(entity)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return unit.Agent(), nil
}

// NewAgentStatusSetter returns a new AgentStatusSetter. The GetAuthFunc will be
// used on each invocation of SetStatus to determine current
// permissions.
func NewAgentStatusSetter(st state.EntityFinder, getCanModify common.GetAuthFunc) *AgentStatusSetter {
	statusSetter := common.NewStatusSetter(
		&AgentEntityFinder{st: st},
		getCanModify)
	return &AgentStatusSetter{statusSetter}
}
