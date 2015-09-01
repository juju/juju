// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/names"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// UserAuthenticator performs authentication for users.
type UserAuthenticator struct {
	AgentAuthenticator
}

const usernameKey = "username"

var _ EntityAuthenticator = (*UserAuthenticator)(nil)

// TODO: MacaroonAuthenticator
// TODO: Issue a macaroon or return pre-generated macaroon -> return ErrDischareReq
//       - where should macaroons be stored? they shouldn't, except in mem (default bakery).
//       - when should they be created?
//         - root key generated on server startup. not reused among replica servers.
//         - macaroon issued on demand, reuse same root key
//       - how do we choose user tag coming in?
//         - special username? placeholder? empty username. need to return with
//           resolved entity in state so some refactoring of authenticators reqd?
// TODO: Verify macaroons -> logged in

// Authenticate authenticates the provided entity and returns an error on authentication failure.
func (u *UserAuthenticator) Authenticate(entityFinder EntityFinder, tag names.Tag, req params.LoginRequest) (state.Entity, error) {
	if tag.Kind() != names.UserTagKind {
		return nil, errors.Errorf("invalid request")
	}
	return u.AgentAuthenticator.Authenticate(entityFinder, tag, req)
}

// DischargeRequiredError is the error returned when a macaroon requires discharging
// to complete authentication.
type DischargeRequiredError struct {
	Macaroon *macaroon.Macaroon
}

// Error implements the error interface.
func (e DischargeRequiredError) Error() string {
	return "discharge required"
}

// MacaroonAuthenticator performs authentication for users using macaroons.
type MacaroonAuthenticator struct {
	Service  *bakery.Service
	Macaroon *macaroon.Macaroon
	Location string
}

var _ EntityAuthenticator = (*MacaroonAuthenticator)(nil)

// Authenticate authenticates the provided entity and returns an error on authentication failure.
func (m *MacaroonAuthenticator) Authenticate(entityFinder EntityFinder, tag names.Tag, req params.LoginRequest) (state.Entity, error) {
	if len(req.Macaroons) == 0 {
		mac := m.Macaroon.Clone()
		err := m.Service.AddCaveat(mac, checkers.TimeBeforeCaveat(time.Now().Add(time.Hour)))
		if err != nil {
			return nil, errors.Annotatef(err, "cannot create macaroon")
		}
		err = m.Service.AddCaveat(mac, checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Location:  m.Location,
				Condition: "is-authenticated-user",
			},
			usernameKey,
		))
		if err != nil {
			return nil, errors.Annotatef(err, "cannot create macaroon")
		}
		return nil, &DischargeRequiredError{mac}
	}

	declared := checkers.InferDeclared(req.Macaroons)
	err := m.Service.Check(req.Macaroons, checkers.New(
		declared,
		checkers.TimeBefore,
	))
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !names.IsValidUser(declared[usernameKey]) {
		return nil, errors.Errorf("%q is an invalid user name", declared[usernameKey])
	}
	entity, err := entityFinder.FindEntity(names.NewUserTag(declared[usernameKey]))
	if errors.IsNotFound(err) {
		return nil, common.ErrBadCreds

	}
	return entity, nil
}
