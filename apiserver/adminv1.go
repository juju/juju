// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v0/bakery"
	"gopkg.in/macaroon-bakery.v0/bakery/checkers"

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
			Locator:  info.IdentityProvider,
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

// MaxMacaroonTTL limits the lifetime of a macaroon issued by Juju for remote
// authentication.
//
// This policy can be further restricted by adding more
// caveats, and is primarily set as a baseline maximum to prevent indefinite
// non-expiring authorization tokens from being issued to clients.
var MaxMacaroonTTL = 2 * 7 * 24 * time.Hour

// RemoteLogin begins a login handshake, returning a ReauthRequest for
// the client to satisfy with a follow-up Login request.
func (a *adminV1) RemoteLogin() (params.ReauthRequest, error) {
	var fail params.ReauthRequest

	info, err := a.srv.state.StateServingInfo()
	if err != nil {
		return fail, errors.Trace(err)
	} else if info.IdentityProvider == nil {
		logger.Debugf("empty credentials, remote identity provider not configured")
		return fail, common.ErrBadCreds
	}

	timeBefore := time.Now().UTC().Add(MaxMacaroonTTL)
	m, err := a.bakeryService.NewMacaroon("", nil, []bakery.Caveat{
		{
			Location:  info.IdentityProvider.Location,
			Condition: "is-authenticated-user",
		},
		{
			Condition: "caveats-include declared-user",
		},
		checkers.TimeBefore(timeBefore),
	})
	if err != nil {
		return fail, errors.Trace(err)
	}

	remoteCreds := authentication.NewRemoteCredentials(m)
	prompt, err := remoteCreds.MarshalText()
	if err != nil {
		return fail, errors.Trace(err)
	}

	return params.ReauthRequest{
		Prompt: string(prompt),
	}, nil
}
