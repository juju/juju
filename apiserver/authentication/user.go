// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// UserAuthenticator performs password based authentication for users.
type UserAuthenticator struct {
	AgentAuthenticator
}

const usernameKey = "username"

var _ EntityAuthenticator = (*UserAuthenticator)(nil)

// Authenticate authenticates the provided entity and returns an error on authentication failure.
func (u *UserAuthenticator) Authenticate(entityFinder EntityFinder, tag names.Tag, req params.LoginRequest) (state.Entity, error) {
	if tag.Kind() != names.UserTagKind {
		return nil, errors.Errorf("invalid request")
	}
	return u.AgentAuthenticator.Authenticate(entityFinder, tag, req)
}

// MacaroonAuthenticator performs authentication for users using macaroons.
// If the authentication fails because provided macaroons are invalid,
// and macaroon authentiction is enabled, it will return a
// *common.DischargeRequiredError holding a macaroon to be
// discharged.
type MacaroonAuthenticator struct {
	Service          *bakery.Service
	Macaroon         *macaroon.Macaroon
	IdentityLocation string
}

var _ EntityAuthenticator = (*MacaroonAuthenticator)(nil)

func (m *MacaroonAuthenticator) newDischargeRequiredError(cause error) error {
	if m.Service == nil || m.Macaroon == nil {
		return errors.Trace(cause)
	}
	mac := m.Macaroon.Clone()
	err := m.Service.AddCaveat(mac, checkers.TimeBeforeCaveat(time.Now().Add(time.Hour)))
	if err != nil {
		return errors.Annotatef(err, "cannot create macaroon")
	}
	err = m.Service.AddCaveat(mac, checkers.NeedDeclaredCaveat(
		checkers.Caveat{
			Location:  m.IdentityLocation,
			Condition: "is-authenticated-user",
		},
		usernameKey,
	))
	if err != nil {
		return errors.Annotatef(err, "cannot create macaroon")
	}
	return &common.DischargeRequiredError{
		Cause:    cause,
		Macaroon: mac,
	}
}

// Authenticate authenticates the provided entity. If there is no macaroon provided, it will
// return a *DischargeRequiredError containing a macaroon that can be used to grant access.
func (m *MacaroonAuthenticator) Authenticate(entityFinder EntityFinder, _ names.Tag, req params.LoginRequest) (state.Entity, error) {
	declared, err := m.Service.CheckAny(req.Macaroons, nil, checkers.New(checkers.TimeBefore))
	if err != nil {
		return nil, m.newDischargeRequiredError(err)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !names.IsValidUser(declared[usernameKey]) {
		return nil, errors.Errorf("%q is an invalid user name", declared[usernameKey])
	}
	entity, err := entityFinder.FindEntity(names.NewUserTag(declared[usernameKey]))
	if errors.IsNotFound(err) {
		return nil, common.ErrBadCreds
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return entity, nil
}
