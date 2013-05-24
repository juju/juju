// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"launchpad.net/juju-core/state/api/params"
	"strings"
)

// Unit represents the state of a service unit.
type Unit struct {
	st   *State
	name string
	doc  params.Unit
}

// UnitTag returns the tag for the
// unit with the given name.
func UnitTag(unitName string) string {
	return "unit-" + strings.Replace(unitName, "/", "-", -1)
}

// Tag returns a name identifying the unit that is safe to use
// as a file name.  The returned name will be different from other
// Tag values returned by any other entities from the same state.
func (u *Unit) Tag() string {
	return UnitTag(u.name)
}

// DeployerTag returns the tag of the agent responsible for deploying
// the unit. If no such entity can be determined, false is returned.
func (u *Unit) DeployerTag() (string, bool) {
	return u.doc.DeployerTag, u.doc.DeployerTag != ""
}

// Refresh refreshes the contents of the Unit from the underlying
// state. TODO(rog) It returns a NotFoundError if the unit has been removed.
func (u *Unit) Refresh() error {
	return u.st.call("Unit", u.name, "Get", nil, &u.doc)
}

// SetPassword sets the password for the unit's agent.
func (u *Unit) SetPassword(password string) error {
	return u.st.call("Unit", u.name, "SetPassword", &params.Password{
		Password: password,
	}, nil)
}
