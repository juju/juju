// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/state"
)

// StatusAPI is the uniter part that deals with setting/getting
// status from different entities, this particular separation from
// base is because we have a shim to support unit/agent split.
type StatusAPI struct {
	agentSetter       *common.StatusSetter
	unitSetter        *common.StatusSetter
	unitGetter        *common.StatusGetter
	applicationSetter *common.ApplicationStatusSetter
	applicationGetter *common.ApplicationStatusGetter
	getCanModify      common.GetAuthFunc
}

// NewStatusAPI creates a new server-side Status setter API facade.
func NewStatusAPI(st *state.State, getCanModify common.GetAuthFunc, leadershipChecker leadership.Checker) *StatusAPI {
	// TODO(fwereade): so *all* of these have exactly the same auth
	// characteristics? I think not.
	unitSetter := common.NewStatusSetter(st, getCanModify)
	unitGetter := common.NewStatusGetter(st, getCanModify)
	applicationSetter := common.NewApplicationStatusSetter(st, getCanModify, leadershipChecker)
	applicationGetter := common.NewApplicationStatusGetter(st, getCanModify, leadershipChecker)
	agentSetter := common.NewStatusSetter(&common.UnitAgentFinder{st}, getCanModify)
	return &StatusAPI{
		agentSetter:       agentSetter,
		unitSetter:        unitSetter,
		unitGetter:        unitGetter,
		applicationSetter: applicationSetter,
		applicationGetter: applicationGetter,
		getCanModify:      getCanModify,
	}
}

// SetStatus will set status for a entities passed in args. If the entity is
// a Unit it will instead set status to its agent, to emulate backwards
// compatibility.
func (s *StatusAPI) SetStatus(args params.SetStatus) (params.ErrorResults, error) {
	return s.SetAgentStatus(args)
}

// SetAgentStatus will set status for agents of Units passed in args, if one
// of the args is not an Unit it will fail.
func (s *StatusAPI) SetAgentStatus(args params.SetStatus) (params.ErrorResults, error) {
	return s.agentSetter.SetStatus(args)
}

// SetUnitStatus sets status for all elements passed in args, the difference
// with SetStatus is that if an entity is a Unit it will set its status instead
// of its agent.
func (s *StatusAPI) SetUnitStatus(args params.SetStatus) (params.ErrorResults, error) {
	return s.unitSetter.SetStatus(args)
}

// SetApplicationStatus sets the status for all the Applications in args if the given Unit is
// the leader.
func (s *StatusAPI) SetApplicationStatus(args params.SetStatus) (params.ErrorResults, error) {
	return s.applicationSetter.SetStatus(args)
}

// UnitStatus returns the workload status information for the unit.
func (s *StatusAPI) UnitStatus(args params.Entities) (params.StatusResults, error) {
	return s.unitGetter.Status(args)
}

// ApplicationStatus returns the status of the Applications and its workloads
// if the given unit is the leader.
func (s *StatusAPI) ApplicationStatus(args params.Entities) (params.ApplicationStatusResults, error) {
	return s.applicationGetter.Status(args)
}
