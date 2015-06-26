// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2/txn"
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
	doc, err := getStatus(u.st, u.globalKey())
	if err != nil {
		return StatusInfo{}, errors.Trace(err)
	}
	// The current health spec says when a hook error occurs, the workload should
	// be in error state, but the state model more correctly records the agent
	// itself as being in error. So we'll do that model translation here.
	if doc.Status == StatusError {
		return StatusInfo{
			Status:  StatusIdle,
			Message: "",
			Data:    map[string]interface{}{},
			Since:   doc.Updated,
		}, nil
	}
	return StatusInfo{
		Status:  doc.Status,
		Message: doc.StatusInfo,
		Data:    doc.StatusData,
		Since:   doc.Updated,
	}, nil
}

// SetStatus sets the status of the unit agent. The optional values
// allow to pass additional helpful status data.
func (u *UnitAgent) SetStatus(status Status, info string, data map[string]interface{}) (err error) {
	oldDoc, err := getStatus(u.st, u.globalKey())
	if IsStatusNotFound(err) {
		logger.Debugf("there is no state for %q yet", u.globalKey())
	} else if err != nil {
		logger.Debugf("cannot get state for %q yet", u.globalKey())
	}

	doc, err := newUnitAgentStatusDoc(status, info, data)
	if err != nil {
		return errors.Trace(err)
	}
	ops := []txn.Op{
		updateStatusOp(u.st, u.globalKey(), doc.statusDoc),
	}
	err = u.st.runTransaction(ops)
	if err != nil {
		return errors.Errorf("cannot set status of unit agent %q: %v", u, onAbort(err, ErrDead))
	}

	if oldDoc.Status != "" {
		if err := updateStatusHistory(oldDoc, u.globalKey(), u.st); err != nil {
			logger.Errorf("could not record status history before change to %q: %v", status, err)
		}
	}

	return nil
}

// StatusHistory returns a slice of at most <size> StatusInfo items
// representing past statuses for this agent.
func (u *UnitAgent) StatusHistory(size int) ([]StatusInfo, error) {
	return statusHistory(size, u.globalKey(), u.st)
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
