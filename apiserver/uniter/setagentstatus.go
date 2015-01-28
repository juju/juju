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
	st state.State
}

func (a *AgentEntityFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	id := tag.Id()
	_, ok := tag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("unsupported tag %T", tag)
	}
	unit, err := a.st.Unit(id)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get unit %q", id)
	}
	return unit.Agent(), nil
}

// NewAgentStatusSetter returns a new AgentStatusSetter. The GetAuthFunc will be
// used on each invocation of SetStatus to determine current
// permissions.
func NewAgentStatusSetter(getCanModify common.GetAuthFunc) *AgentStatusSetter {
	statusSetter := common.NewStatusSetter(
		&AgentEntityFinder{},
		getCanModify)
	return &AgentStatusSetter{statusSetter}
}
