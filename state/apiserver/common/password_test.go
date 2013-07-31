// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	stdtesting "testing"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
)

type passwordSuite struct{}

func TestAll(t *stdtesting.T) {
	TestingT(t)
}

var _ = Suite(&passwordSuite{})

func (*passwordSuite) TestSetPasswords(c *C) {
	st := &fakeAuthState{
		entities: map[string]state.AgentEntity{
			"x0": &fakeEntity{},
			"x1": &fakeEntity{},
			"x2": &fakeEntity{
				err: fmt.Errorf("x2 error"),
			},
			"x3": &fakeEntity{},
		},
	}
	getCanChange := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return tag != "x0"
		}, nil
	}
	pc := common.NewPasswordChanger(st, getCanChange)
	var changes []params.PasswordChange
	for i := 0; i < 4; i++ {
		tag := fmt.Sprintf("x%d", i)
		changes = append(changes, params.PasswordChange{
			Tag:      tag,
			Password: fmt.Sprintf("%spass", tag),
		})
	}
	results, err := pc.SetPasswords(params.PasswordChanges{
		Changes: changes,
	})
	c.Assert(err, IsNil)
	c.Assert(results, DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{&params.Error{Message: "x2 error"}},
			{nil},
		},
	})
	c.Assert(st.entities["x0"].(*fakeEntity).pass, Equals, "")
	c.Assert(st.entities["x1"].(*fakeEntity).pass, Equals, "x1pass")
	c.Assert(st.entities["x1"].(*fakeEntity).mongoPass, Equals, "x1pass")
	c.Assert(st.entities["x2"].(*fakeEntity).pass, Equals, "")
	c.Assert(st.entities["x3"].(*fakeEntity).pass, Equals, "x3pass")
	c.Assert(st.entities["x3"].(*fakeEntity).mongoPass, Equals, "x3pass")
}

func (*passwordSuite) TestSetPasswordsError(c *C) {
	getCanChange := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	pc := common.NewPasswordChanger(&fakeAuthState{}, getCanChange)
	var changes []params.PasswordChange
	for i := 0; i < 4; i++ {
		tag := fmt.Sprintf("x%d", i)
		changes = append(changes, params.PasswordChange{
			Tag:      tag,
			Password: fmt.Sprintf("%spass", tag),
		})
	}
	_, err := pc.SetPasswords(params.PasswordChanges{Changes: changes})
	c.Assert(err, ErrorMatches, "splat")
}

func (*passwordSuite) TestSetPasswordsNoArgsNoError(c *C) {
	getCanChange := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	pc := common.NewPasswordChanger(&fakeAuthState{}, getCanChange)
	result, err := pc.SetPasswords(params.PasswordChanges{})
	c.Assert(err, IsNil)
	c.Assert(result.Results, HasLen, 0)
}

type fakeAuthState struct {
	entities map[string]state.AgentEntity
}

func (st *fakeAuthState) AgentEntity(tag string) (state.AgentEntity, error) {
	if auth, ok := st.entities[tag]; ok {
		return auth, nil
	}
	return nil, errors.NotFoundf("entity %q", tag)
}

type fakeEntity struct {
	err       error
	pass      string
	mongoPass string

	// Any AgentEntity methods we don't implement on fakeEntity
	// will fall back to this and panic.
	state.AgentEntity
}

func (a *fakeEntity) SetPassword(pass string) error {
	if a.err != nil {
		return a.err
	}
	a.pass = pass
	return nil
}

func (a *fakeEntity) SetMongoPassword(pass string) error {
	if a.err != nil {
		return a.err
	}
	a.mongoPass = pass
	return nil
}
