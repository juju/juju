// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
	apiservertesting "github.com/juju/juju/state/apiserver/testing"
	"github.com/juju/names"
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
		x0 := u("x/0")
		x1 := u("x/1")
		x2 := u("x/2")
		x3 := u("x/3")
		x4 := u("x/4")
		x5 := u("x/5")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x1 || tag == x2 || tag == x3 || tag == x4 || tag == x5
		}, nil
	}

	r := common.NewRemover(st, true, getCanModify)
	entities := params.Entities{[]params.Entity{
		{"unit-x-0"}, {"unit-x-1"}, {"unit-x-2"}, {"unit-x-3"}, {"unit-x-4"}, {"unit-x-5"}, {"unit-x-6"},
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

	// Make sure when callEnsureDead is false EnsureDead() doesn't
	// get called.
	r = common.NewRemover(st, false, getCanModify)
	entities = params.Entities{[]params.Entity{{"x0"}, {"x1"}}}
	result, err = r.Remove(entities)
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
