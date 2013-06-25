// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// srvUser serves API methods on a state User.
type srvUser struct {
	root *srvRoot
	u    *state.User
}

// SetPassword sets the user's password.
func (u *srvUser) SetPassword(p params.Password) error {
	return setPassword(u.u, p.Password)
}

// Get retrieves all details of a user.
func (u *srvUser) Get() (params.User, error) {
	return params.User{}, nil
}
