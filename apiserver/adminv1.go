// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

type adminApiV1 struct {
	admin *adminV1
}

// adminV1 is the only object that unlogged-in clients can access. It holds any
// methods that are needed to log in.
type adminV1 struct {
	*admin
}

func newAdminApiV1(srv *Server, root *apiHandler, reqNotifier *requestNotifier) interface{} {
	return &adminApiV1{
		admin: &adminV1{
			&admin{
				srv:         srv,
				root:        root,
				reqNotifier: reqNotifier,
			},
		},
	}
}

// Admin returns an object that provides API access to methods that can be
// called even when not authenticated.
func (r *adminApiV1) Admin(id string) (*adminV1, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return r.admin, nil
}

// Login logs in with the provided credentials.  All subsequent requests on the
// connection will act as the authenticated user.
func (a *adminV1) Login(req params.LoginRequest) (params.LoginResultV1, error) {
	return a.doLogin(req, 1)
}
