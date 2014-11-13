// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/macaroon/bakery"

	"github.com/juju/juju/apiserver/authentication"
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
	bakeryService *bakery.Service
}

func newAdminApiV1(srv *Server, root *apiHandler, reqNotifier *requestNotifier) (interface{}, error) {
	var bakeryService *bakery.Service

	info, err := srv.state.StateServingInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if info.TargetKeyPair != nil && info.IdentityProvider != nil {
		bakeryService, err = bakery.NewService(bakery.NewServiceParams{
			Location: srv.environUUID,
			Key:      info.TargetKeyPair,
			Locator:  info.NewTargetLocator(),
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return &adminApiV1{
		admin: &adminV1{
			admin: &admin{
				srv:         srv,
				root:        root,
				reqNotifier: reqNotifier,
			},
			bakeryService: bakeryService,
		},
	}, nil
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

func isRemoteLoginRequest(req params.LoginRequest) bool {
	// Empty credentials is a request to handshake.
	if req.Credentials == "" {
		return true
	}

	// Otherwise, do we have well-formed remote credentials?
	var remoteCreds authentication.RemoteCredentials
	return remoteCreds.UnmarshalText([]byte(req.Credentials)) == nil
}

// Login logs in with the provided credentials. All subsequent requests on the
// connection will act as the authenticated user.
func (a *adminV1) Login(req params.LoginRequest) (params.LoginResultV1, error) {
	if a.bakeryService != nil && isRemoteLoginRequest(req) {
		return a.doLogin(req, newRemoteCredentialChecker(a.srv.state, a.bakeryService))
	}
	return a.doLogin(req, newLocalCredentialChecker(a.srv.state))
}
