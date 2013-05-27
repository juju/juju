// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// srvUnit serves API methods on a unit.
type srvUnit struct {
	root *srvRoot
	u    *state.Unit
}

// Get retrieves all the details of a unit.
func (u *srvUnit) Get() (params.Unit, error) {
	var ru params.Unit
	ru.DeployerTag, _ = u.u.DeployerTag()
	// TODO add other unit attributes
	return ru, nil
}

// SetPassword sets the unit's password.
func (u *srvUnit) SetPassword(p params.Password) error {
	tag := u.root.user.authenticator().Tag()
	// Allow:
	// - the unit itself.
	// - the machine responsible for unit, if unit is principal
	// - the unit's principal unit, if unit is subordinate
	allow := tag == u.u.Tag()
	if !allow {
		deployerTag, ok := u.u.DeployerTag()
		allow = ok && tag == deployerTag
	}
	if !allow {
		return errPerm
	}
	return setPassword(u.u, p.Password)
}
