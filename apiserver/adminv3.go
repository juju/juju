// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/params"
)

type adminAPIV3 struct {
	*admin
}

func newAdminAPIV3(srv *Server, root *apiHandler, apiObserver observer.Observer) interface{} {
	return &adminAPIV3{
		&admin{
			srv:         srv,
			root:        root,
			apiObserver: apiObserver,
		},
	}
}

// Admin returns an object that provides API access to methods that can be
// called even when not authenticated.
func (r *adminAPIV3) Admin(id string) (*adminAPIV3, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return r, nil
}

// Login logs in with the provided credentials.  All subsequent requests on the
// connection will act as the authenticated user.
func (a *adminAPIV3) Login(req params.LoginRequest) (params.LoginResult, error) {
	return a.login(req, 3)
}

// RedirectInfo returns redirected host information for the model.
// In Juju it always returns an error because the Juju controller
// does not multiplex controllers.
func (a *adminAPIV3) RedirectInfo() (params.RedirectInfoResult, error) {
	return params.RedirectInfoResult{}, fmt.Errorf("not redirected")
}
