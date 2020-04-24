// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/status"
)

// UnitAgent represents the state of an application's unit agent.
type UnitAgent struct {
	st   *State
	tag  names.Tag
	name string
	status.StatusHistoryGetter
}

func newUnitAgent(st *State, tag names.Tag, name string) *UnitAgent {
	unitAgent := &UnitAgent{
		st:   st,
		tag:  tag,
		name: name,
	}

	return unitAgent
}

// String returns the unit agent as string.
func (u *UnitAgent) String() string {
	return u.name
}

// Status returns the status of the unit agent.
func (u *UnitAgent) Status() (status.StatusInfo, error) {
	info, err := getStatus(u.st.db(), u.globalKey(), "agent")
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}
	// The current health spec says when a hook error occurs, the workload should
	// be in error state, but the state model more correctly records the agent
	// itself as being in error. So we'll do that model translation here.
	// TODO(fwereade): this should absolutely not be happpening in the model.
	// TODO: when fixed, also fix code in status.go for UnitAgent and backingUnit.
	if info.Status == status.Error {
		return status.StatusInfo{
			Status:  status.Idle,
			Message: "",
			Data:    map[string]interface{}{},
			Since:   info.Since,
		}, nil
	}
	return info, nil
}

// SetStatus sets the status of the unit agent. The optional values
// allow to pass additional helpful status data.
func (u *UnitAgent) SetStatus(unitAgentStatus status.StatusInfo) (err error) {
	unit, err := u.st.Unit(u.name)
	if errors.IsNotFound(err) {
		return errors.Annotate(errors.NotFoundf("agent"), "cannot set status")
	}
	if err != nil {
		return errors.Trace(err)
	}
	isAssigned := unit.doc.MachineId != ""
	shouldBeAssigned := unit.ShouldBeAssigned()
	isPrincipal := unit.doc.Principal == ""

	switch unitAgentStatus.Status {
	case status.Idle, status.Executing, status.Rebooting, status.Failed:
		if !isAssigned && isPrincipal && shouldBeAssigned {
			return errors.Errorf("cannot set status %q until unit is assigned", unitAgentStatus.Status)
		}
	case status.Error:
		if unitAgentStatus.Message == "" {
			return errors.Errorf("cannot set status %q without info", unitAgentStatus.Status)
		}
	case status.Allocating:
		if isAssigned {
			return errors.Errorf("cannot set status %q as unit is already assigned", unitAgentStatus.Status)
		}
	case status.Running:
		// Only CAAS units (those that require assignment) can have a status of running.
		if shouldBeAssigned {
			return errors.Errorf("cannot set invalid status %q", unitAgentStatus.Status)
		}
	case status.Lost:
		return errors.Errorf("cannot set status %q", unitAgentStatus.Status)
	default:
		return errors.Errorf("cannot set invalid status %q", unitAgentStatus.Status)
	}
	return setStatus(u.st.db(), setStatusParams{
		badge:     "agent",
		globalKey: u.globalKey(),
		status:    unitAgentStatus.Status,
		message:   unitAgentStatus.Message,
		rawData:   unitAgentStatus.Data,
		updated:   timeOrNow(unitAgentStatus.Since, u.st.clock()),
	})
}

// StatusHistory returns a slice of at most filter.Size StatusInfo items
// or items as old as filter.Date or items newer than now - filter.Delta time
// representing past statuses for this agent.
func (u *UnitAgent) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	args := &statusHistoryArgs{
		db:        u.st.db(),
		globalKey: u.globalKey(),
		filter:    filter,
	}
	return statusHistory(args)
}

// unitAgentGlobalKey returns the global database key for the named unit.
func unitAgentGlobalKey(name string) string {
	return "u#" + name
}

// globalKey returns the global database key for the unit.
func (u *UnitAgent) globalKey() string {
	return unitAgentGlobalKey(u.name)
}

// Tag returns a names.Tag identifying this agent's unit.
func (u *UnitAgent) Tag() names.Tag {
	return u.tag
}
