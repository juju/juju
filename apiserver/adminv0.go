// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

// adminApiV0 implements the API that a client first sees when connecting to
// the API. We start serving a different API once the user has logged in.
type adminApiV0 struct {
	admin *adminV0
}

type adminV0 struct {
	*admin
}

func newAdminApiV0(srv *Server, root *apiHandler, reqNotifier *requestNotifier) interface{} {
	return &adminApiV0{
		admin: &adminV0{
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
func (r *adminApiV0) Admin(id string) (*adminV0, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return r.admin, nil
}

// Login logs in with the provided credentials.  All subsequent requests on the
// connection will act as the authenticated user.
func (a *adminV0) Login(c params.Creds) (params.LoginResult, error) {
	var fail params.LoginResult

	resultV1, err := a.doLogin(params.LoginRequest{
		AuthTag:     c.AuthTag,
		Credentials: c.Password,
		Nonce:       c.Nonce,
	}, 0)
	if err != nil {
		return fail, err
	}

	resultV0 := params.LoginResult{
		Servers:    resultV1.Servers,
		EnvironTag: resultV1.EnvironTag,
		Facades:    resultV1.Facades,
	}
	if resultV1.UserInfo != nil {
		resultV0.LastConnection = resultV1.UserInfo.LastConnection
	}
	return resultV0, nil
}
