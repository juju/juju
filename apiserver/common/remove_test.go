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

type removeSuite struct{}

var _ = gc.Suite(&removeSuite{})

type fakeRemover struct {
	state.Entity
	life          state.Life
	errEnsureDead error
	errRemove     error
	fetchError
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
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeRemover{life: state.Dying, errEnsureDead: fmt.Errorf("x0 EnsureDead fails")},
			u("x/1"): &fakeRemover{life: state.Dying, errRemove: fmt.Errorf("x1 Remove fails")},
			u("x/2"): &fakeRemover{life: state.Alive},
			u("x/3"): &fakeRemover{life: state.Dying},
			u("x/4"): &fakeRemover{life: state.Dead},
			u("x/5"): &fakeRemover{fetchError: "x5 error"},
		},
	}
	getCanModify := func() (common.AuthFunc, error) {
		u0 := u("x/0")
		u1 := u("x/1")
		u2 := u("x/2")
		u3 := u("x/3")
		u5 := u("x/5")
		return func(tag names.Tag) bool {
			return tag == u0 || tag == u1 || tag == u2 || tag == u3 || tag == u5
		}, nil
	}

	r := common.NewRemover(st, true, getCanModify)
	entities := params.Entities{[]params.Entity{
		{"unit-x-0"}, {"unit-x-1"}, {"unit-x-2"}, {"unit-x-3"}, {"unit-x-4"}, {"unit-x-5"}, {"unit-x-6"},
	}}
	result, err := r.Remove(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: "x0 EnsureDead fails"}},
			{&params.Error{Message: "x1 Remove fails"}},
			{&params.Error{Message: `cannot remove entity "unit-x-2": still alive`}},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{&params.Error{Message: "x5 error"}},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Make sure when callEnsureDead is false EnsureDead() doesn't
	// get called.
	r = common.NewRemover(st, false, getCanModify)
	entities = params.Entities{[]params.Entity{{"unit-x-0"}, {"unit-x-1"}}}
	result, err = r.Remove(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{&params.Error{Message: "x1 Remove fails"}},
		},
	})
}

func (*removeSuite) TestRemoveError(c *gc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	r := common.NewRemover(&fakeState{}, true, getCanModify)
	_, err := r.Remove(params.Entities{[]params.Entity{{"x0"}}})
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*removeSuite) TestRemoveNoArgsNoError(c *gc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	r := common.NewRemover(&fakeState{}, true, getCanModify)
	result, err := r.Remove(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
