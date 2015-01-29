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

func getUnitFromId(st state.State, id string) (state.AgentUnit, error) {
	return st.Unit(id)
}

var unitFromId = getUnitFromId

func (a *AgentEntityFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	_, ok := tag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("unsupported tag %T", tag)
	}

	id := tag.Id()
	unit, err := unitFromId(a.st, id)
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
