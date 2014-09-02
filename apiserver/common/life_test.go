// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

type lifeSuite struct{}

var _ = gc.Suite(&lifeSuite{})

type fakeLifer struct {
	state.Entity
	life state.Life
	fetchError
}

func (l *fakeLifer) Life() state.Life {
	return l.life
}

func (*lifeSuite) TestLife(c *gc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeLifer{life: state.Alive},
			u("x/1"): &fakeLifer{life: state.Dying},
			u("x/2"): &fakeLifer{life: state.Dead},
			u("x/3"): &fakeLifer{fetchError: "x3 error"},
		},
	}
	getCanRead := func() (common.AuthFunc, error) {
		x0 := u("x/0")
		x2 := u("x/2")
		x3 := u("x/3")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x2 || tag == x3
		}, nil
	}
	lg := common.NewLifeGetter(st, getCanRead)
	entities := params.Entities{[]params.Entity{
		{"unit-x-0"}, {"unit-x-1"}, {"unit-x-2"}, {"unit-x-3"}, {"unit-x-4"},
	}}
	results, err := lg.Life(entities)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: params.Alive},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: params.Dead},
			{Error: &params.Error{Message: "x3 error"}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (*lifeSuite) TestLifeError(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	lg := common.NewLifeGetter(&fakeState{}, getCanRead)
	_, err := lg.Life(params.Entities{[]params.Entity{{"x0"}}})
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*lifeSuite) TestLifeNoArgsNoError(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	lg := common.NewLifeGetter(&fakeState{}, getCanRead)
	result, err := lg.Life(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
