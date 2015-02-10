// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/names"
	"gopkg.in/mgo.v2/txn"
)

// UnitAgent represents the state of a service unit agent.
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

// Status returns the status of the unit.
func (u *UnitAgent) Status() (status Status, info string, data map[string]interface{}, err error) {
	doc, err := getStatus(u.st, u.globalKey())
	if err != nil {
		return "", "", nil, err
	}
	status = doc.Status
	info = doc.StatusInfo
	data = doc.StatusData
	return
}

// SetStatus sets the status of the unit agent. The optional values
// allow to pass additional helpful status data.
func (u *UnitAgent) SetStatus(status Status, info string, data map[string]interface{}) error {
	doc, err := newUnitAgentStatusDoc(status, info, data)
	if err != nil {
		return err
	}
	ops := []txn.Op{
		updateStatusOp(u.st, u.globalKey(), doc.statusDoc),
	}
	err = u.st.runTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot set status of unit agent %q: %v", u, onAbort(err, ErrDead))
	}
	return nil
}

// unitGlobalKey returns the global database key for the named unit.
func unitAgentGlobalKey(name string) string {
	return "u#" + name
}

// globalKey returns the global database key for the unit.
func (u *UnitAgent) globalKey() string {
	return unitAgentGlobalKey(u.name)
}

// Tag returns a name identifying this agent's unit.
func (u *UnitAgent) Tag() names.Tag {
	return u.tag
}
