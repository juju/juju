// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/names"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
)

// authContext holds authentication context shared
// between all API endpoints.
type authContext struct {
	// bakeryService holds the service that is
	// used to verify macaroon authorization.
	// It will be nil if no identity URL has been configured.
	bakeryService *bakery.Service

	// macaroon guards macaroon-authentication-based access
	// to the APIs.
	macaroon *macaroon.Macaroon

	// identityURL holds the URL of the trusted third party
	// that is used to address the is-authenticated-user
	// third party caveat to.
	identityURL string
}

// authenticatorForTag returns the authenticator appropriate
// to use for a login with the given possibly-nil tag.
func (ctxt *authContext) authenticatorForTag(tag names.Tag) (authentication.EntityAuthenticator, error) {
	if tag == nil {
		return &authentication.MacaroonAuthenticator{
			Service:          ctxt.bakeryService,
			Macaroon:         ctxt.macaroon,
			IdentityLocation: ctxt.identityURL,
		}, nil
	}

	switch tag.Kind() {
	case names.UnitTagKind, names.MachineTagKind:
		return &authentication.AgentAuthenticator{}, nil
	case names.UserTagKind:
		return &authentication.UserAuthenticator{}, nil
	}
	return nil, common.ErrBadRequest
}
