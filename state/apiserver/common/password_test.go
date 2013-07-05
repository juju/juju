// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	stdtesting "testing"
)

type passwordSuite struct{}

func TestAll(t *stdtesting.T) {
	TestingT(t)
}

var _ = Suite(passwordSuite{})

func (passwordSuite) TestSetPasswords(c *C) {
	st := &fakeState{
		entities: map[string]state.TaggedAuthenticator{
			"x0": &fakeAuthenticator{},
			"x1": &fakeAuthenticator{},
			"x2": &fakeAuthenticator{
				err: fmt.Errorf("x2 error"),
			},
			"x3": &fakeAuthenticatorWithMongoPass{},
		},
	}
	pc := common.NewMockPasswordChanger(st, func(tag string) bool {
		return tag != "x0"
	})
	var changes []params.PasswordChange
	for i := 0; i < 4; i++ {
		tag := fmt.Sprintf("x%d", i)
		changes = append(changes, params.PasswordChange{
			Tag:      tag,
			Password: fmt.Sprintf("%spass", tag),
		})
	}
	results := pc.SetPasswords(params.PasswordChanges{
		Changes: changes,
	})
	c.Assert(results, DeepEquals, params.ErrorResults{
		Errors: []*params.Error{{
			Message: "permission denied",
			Code:    params.CodeUnauthorized,
		},
			nil, {
				Message: "x2 error",
			},
			nil,
		}})
	c.Assert(st.entities["x0"].(*fakeAuthenticator).pass, Equals, "")
	c.Assert(st.entities["x1"].(*fakeAuthenticator).pass, Equals, "x1pass")
	c.Assert(st.entities["x2"].(*fakeAuthenticator).pass, Equals, "")
	c.Assert(st.entities["x3"].(*fakeAuthenticatorWithMongoPass).pass, Equals, "x3pass")
	c.Assert(st.entities["x3"].(*fakeAuthenticatorWithMongoPass).mongoPass, Equals, "x3pass")
}

type fakeState struct {
	entities map[string]state.TaggedAuthenticator
}

func (st *fakeState) Authenticator(tag string) (state.TaggedAuthenticator, error) {
	if auth, ok := st.entities[tag]; ok {
		return auth, nil
	}
	return nil, errors.NotFoundf("entity %q", tag)
}

type fakeAuthenticator struct {
	err  error
	pass string
}

func (a *fakeAuthenticator) Tag() string {
	panic("Tag not implemented")
}

func (a *fakeAuthenticator) Refresh() error {
	panic("Refresh not implemented")
}

func (a *fakeAuthenticator) PasswordValid(string) bool {
	panic("PasswordValid not implemented")
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
