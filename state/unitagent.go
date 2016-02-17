// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/juju/status"
	"github.com/juju/names"
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
	if info.Status == status.StatusError {
		return status.StatusInfo{
			Status:  status.StatusIdle,
			Message: "",
			Data:    map[string]interface{}{},
			Since:   info.Since,
		}, nil
	}
	return info, nil
}

// SetStatus sets the status of the unit agent. The optional values
// allow to pass additional helpful status data.
func (u *UnitAgent) SetStatus(unitAgentStatus status.Status, info string, data map[string]interface{}) (err error) {
	switch unitAgentStatus {
	case status.StatusIdle, status.StatusExecuting, status.StatusRebooting, status.StatusFailed:
	case status.StatusError:
		if info == "" {
			return errors.Errorf("cannot set status %q without info", unitAgentStatus)
		}
	case status.StatusAllocating, status.StatusLost:
		return errors.Errorf("cannot set status %q", unitAgentStatus)
	default:
		return errors.Errorf("cannot set invalid status %q", unitAgentStatus)
	}
	return setStatus(u.st, setStatusParams{
		badge:     "agent",
		globalKey: u.globalKey(),
		status:    unitAgentStatus,
		message:   info,
		rawData:   data,
	})
}

// StatusHistory returns a slice of at most <size> StatusInfo items
// representing past statuses for this agent.
func (u *UnitAgent) StatusHistory(size int) ([]status.StatusInfo, error) {
	return statusHistory(u.st, u.globalKey(), size)
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
