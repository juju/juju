// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import "launchpad.net/juju-core/state"

var (
	ServerError       = serverError
	ErrBadId          = errBadId
	ErrBadVersion     = errBadVersion
	ErrBadCreds       = errBadCreds
	ErrPerm           = errPerm
	ErrNotLoggedIn    = errNotLoggedIn
	ErrUnknownWatcher = errUnknownWatcher
	ErrStoppedWatcher = errStoppedWatcher
)

type SrvMachiner struct {
	*srvMachiner
}

func NewMachiner(st *state.State, auth Authorizer) *SrvMachiner {
	return &SrvMachiner{&srvMachiner{st, auth}}
}
