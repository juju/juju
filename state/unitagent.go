// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/status"
)

// UnitAgent represents the state of a service's unit agent.
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
	info, err := getStatus(u.st, u.globalKey(), "agent")
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}
	// The current health spec says when a hook error occurs, the workload should
	// be in error state, but the state model more correctly records the agent
	// itself as being in error. So we'll do that model translation here.
	// TODO(fwereade): this should absolutely not be happpening in the model.
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
	switch unitAgentStatus.Status {
	case status.Idle, status.Executing, status.Rebooting, status.Failed:
	case status.Error:
		if unitAgentStatus.Message == "" {
			return errors.Errorf("cannot set status %q without info", unitAgentStatus.Status)
		}
	case status.Allocating, status.Lost:
		return errors.Errorf("cannot set status %q", unitAgentStatus.Status)
	default:
		return errors.Errorf("cannot set invalid status %q", unitAgentStatus.Status)
	}
	return setStatus(u.st, setStatusParams{
		badge:     "agent",
		globalKey: u.globalKey(),
		status:    unitAgentStatus.Status,
		message:   unitAgentStatus.Message,
		rawData:   unitAgentStatus.Data,
		updated:   unitAgentStatus.Since,
	})
}

// StatusHistory returns a slice of at most filter.Size StatusInfo items
// or items as old as filter.Date or items newer than now - filter.Delta time
// representing past statuses for this agent.
func (u *UnitAgent) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	args := &statusHistoryArgs{
		st:        u.st,
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
