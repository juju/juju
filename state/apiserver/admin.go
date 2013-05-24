// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import "launchpad.net/juju-core/state/api/params"

// srvAdmin is the only object that unlogged-in
// clients can access. It holds any methods
// that are needed to log in.
type srvAdmin struct {
	root *srvRoot
}

// Login logs in with the provided credentials.
// All subsequent requests on the connection will
// act as the authenticated user.
func (a *srvAdmin) Login(c params.Creds) error {
	return a.root.user.login(a.root.srv.state, c.AuthTag, c.Password)
}
