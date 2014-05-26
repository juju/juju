// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
)

type passwordSuite struct{}

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

// fakeUnitAuthenticator simulates a unit entity.
type fakeUnitAuthenticator struct {
	fakeAuthenticator
	mongoPass string
}

func (a *fakeUnitAuthenticator) Tag() string {
	return "fake"
}

func (a *fakeUnitAuthenticator) SetMongoPassword(pass string) error {
	if a.err != nil {
		return a.err
	}
	a.mongoPass = pass
	return nil
}

// fakeMachineAuthenticator simulates a machine entity.
type fakeMachineAuthenticator struct {
	fakeUnitAuthenticator
	jobs []state.MachineJob
}

func (a *fakeMachineAuthenticator) Jobs() []state.MachineJob {
	return a.jobs
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
			"x4": &fakeUnitAuthenticator{},
			"x5": &fakeMachineAuthenticator{jobs: []state.MachineJob{state.JobHostUnits}},
			"x6": &fakeMachineAuthenticator{jobs: []state.MachineJob{state.JobManageEnviron}},
		},
	}
	getCanChange := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return tag != "x0"
		}, nil
	}
	pc := common.NewPasswordChanger(st, getCanChange)
	var changes []params.EntityPassword
	for i := 0; i < len(st.entities); i++ {
		tag := fmt.Sprintf("x%d", i)
		changes = append(changes, params.EntityPassword{
			Tag:      tag,
			Password: fmt.Sprintf("%spass", tag),
		})
	}
	results, err := pc.SetPasswords(params.EntityPasswords{
		Changes: changes,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{&params.Error{Message: "x2 error"}},
			{&params.Error{Message: "x3 error"}},
			{nil},
			{nil},
			{nil},
		},
	})
	c.Check(st.entities["x0"].(*fakeAuthenticator).pass, gc.Equals, "")
	c.Check(st.entities["x1"].(*fakeAuthenticator).pass, gc.Equals, "x1pass")
	c.Check(st.entities["x2"].(*fakeAuthenticator).pass, gc.Equals, "")
	c.Check(st.entities["x4"].(*fakeUnitAuthenticator).pass, gc.Equals, "x4pass")
	c.Check(st.entities["x4"].(*fakeUnitAuthenticator).mongoPass, gc.Equals, "")
	c.Check(st.entities["x5"].(*fakeMachineAuthenticator).pass, gc.Equals, "x5pass")
	c.Check(st.entities["x5"].(*fakeMachineAuthenticator).mongoPass, gc.Equals, "")
	c.Check(st.entities["x6"].(*fakeMachineAuthenticator).pass, gc.Equals, "x6pass")
	c.Check(st.entities["x6"].(*fakeMachineAuthenticator).mongoPass, gc.Equals, "x6pass")
}

func (*passwordSuite) TestSetPasswordsError(c *gc.C) {
	getCanChange := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	pc := common.NewPasswordChanger(&fakeState{}, getCanChange)
	var changes []params.EntityPassword
	for i := 0; i < 4; i++ {
		tag := fmt.Sprintf("x%d", i)
		changes = append(changes, params.EntityPassword{
			Tag:      tag,
			Password: fmt.Sprintf("%spass", tag),
		})
	}
	_, err := pc.SetPasswords(params.EntityPasswords{Changes: changes})
	c.Assert(err, gc.ErrorMatches, "splat")
}

func (*passwordSuite) TestSetPasswordsNoArgsNoError(c *gc.C) {
	getCanChange := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	pc := common.NewPasswordChanger(&fakeState{}, getCanChange)
	result, err := pc.SetPasswords(params.EntityPasswords{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
