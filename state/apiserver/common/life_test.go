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
)

type lifeSuite struct{}

var _ = Suite(&lifeSuite{})

func (*lifeSuite) TestLife(c *C) {
	st := &fakeLifeState{
		entities: map[string]*fakeLifer{
			"x0": &fakeLifer{life: state.Alive},
			"x1": &fakeLifer{life: state.Dying},
			"x2": &fakeLifer{life: state.Dead},
			"x3": &fakeLifer{err: fmt.Errorf("x3 error")},
		},
	}
	getCanRead := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			switch tag {
			case "x0", "x2", "x3":
				return true
			}
			return false
		}, nil
	}
	lg := common.NewMockLifeGetter(st, getCanRead)
	entities := params.Entities{[]params.Entity{
		{"x0"}, {"x1"}, {"x2"}, {"x3"}, {"x4"},
	}}
	results, err := lg.Life(entities)
	c.Assert(err, IsNil)
	unauth := &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	c.Assert(results, DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: params.Alive},
			{Error: unauth},
			{Life: params.Dead},
			{Error: &params.Error{
				Message: "x3 error",
			}},
			{Error: unauth},
		},
	})
}

func (*lifeSuite) TestLifeError(c *C) {
	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	lg := common.NewMockLifeGetter(&fakeLifeState{}, getCanRead)
	_, err := lg.Life(params.Entities{})
	c.Assert(err, ErrorMatches, "pow")
}

type fakeLifeState struct {
	entities map[string]*fakeLifer
}

func (st *fakeLifeState) Lifer(tag string) (state.Lifer, error) {
	if lifer, ok := st.entities[tag]; ok {
		if lifer.err != nil {
			return nil, lifer.err
		}
		return lifer, nil
	}
	return nil, errors.NotFoundf("entity %q", tag)
}

type fakeLifer struct {
	life state.Life
	err  error
}

func (l *fakeLifer) Life() state.Life {
	return l.life
}
