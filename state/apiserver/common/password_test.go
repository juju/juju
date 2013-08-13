// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
)

type passwordSuite struct{}

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&passwordSuite{})

type entityWithError interface {
	state.Entity
	error() error
}

type fakeState struct {
	entities map[string]entityWithError
}

func (st *fakeState) FindEntity(tag string) (state.Entity, error) {
	entity, ok := st.entities[tag]
	if !ok {
		return nil, errors.NotFoundf("entity %q", tag)
	}
	if err := entity.error(); err != nil {
		return nil, err
	}
	return entity, nil
}

type fetchError string

func (f fetchError) error() error {
	if f == "" {
		return nil
	}
	return fmt.Errorf("%s", string(f))
}

type fakeAuthenticator struct {
	// Any Authenticator methods we don't implement on fakeAuthenticator
	// will fall back to this and panic because it's always nil.
	state.Authenticator
	state.Entity
	err  error
	pass string
	fetchError
}

func (a *fakeAuthenticator) SetPassword(pass string) error {
	if a.err != nil {
		return a.err
	}
	a.pass = pass
	return nil
}

type fakeAuthenticatorWithMongoPass struct {
	fakeAuthenticator
	mongoPass string
}

func (a *fakeAuthenticatorWithMongoPass) SetMongoPassword(pass string) error {
	if a.err != nil {
		return a.err
	}
	a.mongoPass = pass
	return nil
}

func (*passwordSuite) TestSetPasswords(c *gc.C) {
	st := &fakeState{
		entities: map[string]entityWithError{
			"x0": &fakeAuthenticator{},
			"x1": &fakeAuthenticator{},
			"x2": &fakeAuthenticator{
				err: fmt.Errorf("x2 error"),
			},
			"x3": &fakeAuthenticator{
				fetchError: "x3 error",
			},
			"x4": &fakeAuthenticatorWithMongoPass{},
		},
	}
	getCanChange := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return tag != "x0"
		}, nil
	}
	pc := common.NewPasswordChanger(st, getCanChange)
	var changes []params.PasswordChange
	for i := 0; i < 5; i++ {
		tag := fmt.Sprintf("x%d", i)
		changes = append(changes, params.PasswordChange{
			Tag:      tag,
			Password: fmt.Sprintf("%spass", tag),
		})
	}
	results, err := pc.SetPasswords(params.PasswordChanges{
		Changes: changes,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{&params.Error{Message: "x2 error"}},
			{&params.Error{Message: "x3 error"}},
			{nil},
		},
	})
	c.Assert(st.entities["x0"].(*fakeAuthenticator).pass, gc.Equals, "")
	c.Assert(st.entities["x1"].(*fakeAuthenticator).pass, gc.Equals, "x1pass")
	c.Assert(st.entities["x2"].(*fakeAuthenticator).pass, gc.Equals, "")
	c.Assert(st.entities["x4"].(*fakeAuthenticatorWithMongoPass).pass, gc.Equals, "x4pass")
	c.Assert(st.entities["x4"].(*fakeAuthenticatorWithMongoPass).mongoPass, gc.Equals, "x4pass")
}

func (*passwordSuite) TestSetPasswordsError(c *gc.C) {
	getCanChange := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	pc := common.NewPasswordChanger(&fakeState{}, getCanChange)
	var changes []params.PasswordChange
	for i := 0; i < 4; i++ {
		tag := fmt.Sprintf("x%d", i)
		changes = append(changes, params.PasswordChange{
			Tag:      tag,
			Password: fmt.Sprintf("%spass", tag),
		})
	}
	_, err := pc.SetPasswords(params.PasswordChanges{Changes: changes})
	c.Assert(err, gc.ErrorMatches, "splat")
}

func (*passwordSuite) TestSetPasswordsNoArgsNoError(c *gc.C) {
	getCanChange := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	pc := common.NewPasswordChanger(&fakeState{}, getCanChange)
	result, err := pc.SetPasswords(params.PasswordChanges{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
