// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
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
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeDeadEnsurer{life: state.Dying, err: fmt.Errorf("x0 fails")},
			u("x/1"): &fakeDeadEnsurer{life: state.Alive},
			u("x/2"): &fakeDeadEnsurer{life: state.Dying},
			u("x/3"): &fakeDeadEnsurer{life: state.Dead},
			u("x/4"): &fakeDeadEnsurer{fetchError: "x4 error"},
		},
	}
	getCanModify := func() (common.AuthFunc, error) {
		x0 := u("x/0")
		x1 := u("x/1")
		x2 := u("x/2")
		x4 := u("x/4")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x1 || tag == x2 || tag == x4
		}, nil
	}

	d := common.NewDeadEnsurer(st, getCanModify)
	entities := params.Entities{[]params.Entity{
		{"unit-x-0"}, {"unit-x-1"}, {"unit-x-2"}, {"unit-x-3"}, {"unit-x-4"}, {"unit-x-5"},
	}}
	result, err := d.EnsureDead(entities)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
