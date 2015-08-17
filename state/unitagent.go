// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"
)

// UnitAgent represents the state of a service's unit agent.
type UnitAgent struct {
	st   *State
	tag  names.Tag
	name string
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
func (u *UnitAgent) Status() (StatusInfo, error) {
	info, err := getStatus(u.st, u.globalKey(), "agent")
	if err != nil {
		return StatusInfo{}, errors.Trace(err)
	}
	// The current health spec says when a hook error occurs, the workload should
	// be in error state, but the state model more correctly records the agent
	// itself as being in error. So we'll do that model translation here.
	// TODO(fwereade): this should absolutely not be happpening in the model.
	if info.Status == StatusError {
		return StatusInfo{
			Status:  StatusIdle,
			Message: "",
			Data:    map[string]interface{}{},
			Since:   info.Since,
		}, nil
	}
	return info, nil
}

// SetStatus sets the status of the unit agent. The optional values
// allow to pass additional helpful status data.
func (u *UnitAgent) SetStatus(status Status, info string, data map[string]interface{}) (err error) {
	switch status {
	case StatusIdle, StatusExecuting, StatusRebooting, StatusFailed:
	case StatusError:
		if info == "" {
			return errors.Errorf("cannot set status %q without info", status)
		}
	case StatusAllocating, StatusLost:
		return errors.Errorf("cannot set status %q", status)
	default:
		return errors.Errorf("cannot set invalid status %q", status)
	}
	return setStatus(u.st, setStatusParams{
		badge:     "agent",
		globalKey: u.globalKey(),
		status:    status,
		message:   info,
		rawData:   data,
	})
}

// StatusHistory returns a slice of at most <size> StatusInfo items
// representing past statuses for this agent.
func (u *UnitAgent) StatusHistory(size int) ([]StatusInfo, error) {
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
