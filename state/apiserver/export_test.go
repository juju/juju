// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import "fmt"

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

type SrvRoot struct {
	srvRoot
}

func ServerLoginAndGetRoot(srv *Server, tag, password string) (*SrvRoot, error) {
	if srv.root != nil {
		if err := srv.root.user.login(srv.state, tag, password); err != nil {
			return nil, err
		}
		return &SrvRoot{*srv.root}, nil
	}
	return nil, fmt.Errorf("server root not initialized")
}
