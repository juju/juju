// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
)

type removeSuite struct{}

var _ = gc.Suite(&removeSuite{})

type fakeRemoverState struct {
	entities map[string]*fakeRemover
}

func (st *fakeRemoverState) Remover(tag string) (state.Remover, error) {
	if remover, ok := st.entities[tag]; ok {
		if remover.err != nil {
			return nil, remover.err
		}
		return remover, nil
	}
	return nil, errors.NotFoundf("entity %q", tag)
}

type fakeRemover struct {
	life          state.Life
	err           error
	errEnsureDead error
	errRemove     error
}

func (r *fakeRemover) Tag() string {
	panic("not needed")
}

func (r *fakeRemover) EnsureDead() error {
	return r.errEnsureDead
}

func (r *fakeRemover) Remove() error {
	return r.errRemove
}

func (r *fakeRemover) Life() state.Life {
	return r.life
}

func (*removeSuite) TestRemove(c *gc.C) {
	st := &fakeRemoverState{
		entities: map[string]*fakeRemover{
			"x0": {life: state.Dying, errEnsureDead: fmt.Errorf("x0 EnsureDead fails")},
			"x1": {life: state.Dying, errRemove: fmt.Errorf("x1 Remove fails")},
			"x2": {life: state.Alive},
			"x3": {life: state.Dying},
			"x4": {life: state.Dead},
			"x5": {life: state.Dead, err: fmt.Errorf("x5 error")},
		},
	}
	getCanModify := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			switch tag {
			case "x0", "x1", "x2", "x3", "x5":
				return true
			}
			return false
		}, nil
	}
	r := common.NewRemover(st, getCanModify)
	entities := params.Entities{[]params.Entity{
		{"x0"}, {"x1"}, {"x2"}, {"x3"}, {"x4"}, {"x5"}, {"x6"},
	}}
	result, err := r.Remove(entities)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: "x0 EnsureDead fails"}},
			{&params.Error{Message: "x1 Remove fails"}},
			{&params.Error{Message: `cannot remove entity "x2": still alive`}},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{&params.Error{Message: "x5 error"}},
			{apiservertesting.ErrUnauthorized},
		},
	})
}

func (*removeSuite) TestRemoveError(c *gc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	r := common.NewRemover(&fakeRemoverState{}, getCanModify)
	_, err := r.Remove(params.Entities{[]params.Entity{{"x0"}}})
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*removeSuite) TestRemoveNoArgsNoError(c *gc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	r := common.NewRemover(&fakeRemoverState{}, getCanModify)
	result, err := r.Remove(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
