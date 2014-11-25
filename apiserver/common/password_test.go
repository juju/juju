// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
)

type passwordSuite struct{}

var _ = gc.Suite(&passwordSuite{})

type entityWithError interface {
	state.Entity
	error() error
}

type fakeState struct {
	entities map[names.Tag]entityWithError
}

func (st *fakeState) FindEntity(tag names.Tag) (state.Entity, error) {
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

func (a *fakeUnitAuthenticator) Tag() names.Tag {
	return names.NewUnitTag("fake/0")
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

func (a *fakeMachineAuthenticator) Tag() names.Tag {
	return names.NewMachineTag("0")
}

func (*passwordSuite) TestSetPasswords(c *gc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeAuthenticator{},
			u("x/1"): &fakeAuthenticator{},
			u("x/2"): &fakeAuthenticator{
				err: fmt.Errorf("x2 error"),
			},
			u("x/3"): &fakeAuthenticator{
				fetchError: "x3 error",
			},
			u("x/4"): &fakeUnitAuthenticator{},
			u("x/5"): &fakeMachineAuthenticator{jobs: []state.MachineJob{state.JobHostUnits}},
			u("x/6"): &fakeMachineAuthenticator{jobs: []state.MachineJob{state.JobManageEnviron}},
		},
	}
	getCanChange := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag != names.NewUnitTag("x/0")
		}, nil
	}
	pc := common.NewPasswordChanger(st, getCanChange)
	var changes []params.EntityPassword
	for i := 0; i < len(st.entities); i++ {
		tag := fmt.Sprintf("unit-x-%d", i)
		changes = append(changes, params.EntityPassword{
			Tag:      tag,
			Password: fmt.Sprintf("%spass", tag),
		})
	}
	results, err := pc.SetPasswords(params.EntityPasswords{
		Changes: changes,
	})
	c.Assert(err, jc.ErrorIsNil)
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
	c.Check(st.entities[u("x/0")].(*fakeAuthenticator).pass, gc.Equals, "")
	c.Check(st.entities[u("x/1")].(*fakeAuthenticator).pass, gc.Equals, "unit-x-1pass")
	c.Check(st.entities[u("x/2")].(*fakeAuthenticator).pass, gc.Equals, "")
	c.Check(st.entities[u("x/4")].(*fakeUnitAuthenticator).pass, gc.Equals, "unit-x-4pass")
	c.Check(st.entities[u("x/4")].(*fakeUnitAuthenticator).mongoPass, gc.Equals, "")
	c.Check(st.entities[u("x/5")].(*fakeMachineAuthenticator).pass, gc.Equals, "unit-x-5pass")
	c.Check(st.entities[u("x/5")].(*fakeMachineAuthenticator).mongoPass, gc.Equals, "")
	c.Check(st.entities[u("x/6")].(*fakeMachineAuthenticator).pass, gc.Equals, "unit-x-6pass")
	c.Check(st.entities[u("x/6")].(*fakeMachineAuthenticator).mongoPass, gc.Equals, "unit-x-6pass")
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
