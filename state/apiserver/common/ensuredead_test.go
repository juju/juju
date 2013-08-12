// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
)

type deadEnsurerSuite struct{}

var _ = gc.Suite(&deadEnsurerSuite{})

type fakeDeadEnsurer struct {
	state.Entity
	life state.Life
	err  error
	fetchError
}

func (e *fakeDeadEnsurer) EnsureDead() error {
	return e.err
}

func (e *fakeDeadEnsurer) Life() state.Life {
	return e.life
}

func (*deadEnsurerSuite) TestEnsureDead(c *gc.C) {
	st := &fakeState{
		entities: map[string]entityWithError{
			"x0": &fakeDeadEnsurer{life: state.Dying, err: fmt.Errorf("x0 fails")},
			"x1": &fakeDeadEnsurer{life: state.Alive},
			"x2": &fakeDeadEnsurer{life: state.Dying},
			"x3": &fakeDeadEnsurer{life: state.Dead},
			"x4": &fakeDeadEnsurer{fetchError: "x4 error"},
		},
	}
	getCanModify := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			switch tag {
			case "x0", "x1", "x2", "x4":
				return true
			}
			return false
		}, nil
	}
	d := common.NewDeadEnsurer(st, getCanModify)
	entities := params.Entities{[]params.Entity{
		{"x0"}, {"x1"}, {"x2"}, {"x3"}, {"x4"}, {"x5"},
	}}
	result, err := d.EnsureDead(entities)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: "x0 fails"}},
			{nil},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{&params.Error{Message: "x4 error"}},
			{apiservertesting.ErrUnauthorized},
		},
	})
}

func (*deadEnsurerSuite) TestEnsureDeadError(c *gc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	d := common.NewDeadEnsurer(&fakeState{}, getCanModify)
	_, err := d.EnsureDead(params.Entities{[]params.Entity{{"x0"}}})
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*removeSuite) TestEnsureDeadNoArgsNoError(c *gc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	d := common.NewDeadEnsurer(&fakeState{}, getCanModify)
	result, err := d.EnsureDead(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
