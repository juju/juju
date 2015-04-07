// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

type adminApiV2 struct {
	*admin
}

func newAdminApiV2(srv *Server, root *apiHandler, reqNotifier *requestNotifier) interface{} {
	return &adminApiV2{
		&admin{
			srv:         srv,
			root:        root,
			reqNotifier: reqNotifier,
		},
	}
}

// Admin returns an object that provides API access to methods that can be
// called even when not authenticated.
func (r *adminApiV2) Admin(id string) (*adminApiV2, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return r, nil
}

// Login logs in with the provided credentials.  All subsequent requests on the
// connection will act as the authenticated user.
func (a *adminApiV2) Login(req params.LoginRequest) (params.LoginResultV1, error) {
	return a.doLogin(req, 2)
}
