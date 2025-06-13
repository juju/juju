// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type instanceIdGetterSuite struct{}

var _ = gc.Suite(&instanceIdGetterSuite{})

type fakeInstanceIdGetter struct {
	state.Entity
	instanceId string
	err        string
	fetchError
}

func (f *fakeInstanceIdGetter) InstanceId() (instance.Id, error) {
	if f.err != "" {
		return "", errors.New(f.err)
	}
	return instance.Id(f.instanceId), nil
}

func (*instanceIdGetterSuite) TestInstanceId(c *gc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeInstanceIdGetter{instanceId: "foo"},
			u("x/1"): &fakeInstanceIdGetter{instanceId: "bar"},
			u("x/2"): &fakeInstanceIdGetter{instanceId: "baz", err: "x2 error"},
			u("x/3"): &fakeInstanceIdGetter{fetchError: "x3 error"},
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
	ig := common.NewInstanceIdGetter(st, getCanRead)
	entities := params.Entities{[]params.Entity{
		{"unit-x-0"}, {"unit-x-1"}, {"unit-x-2"}, {"unit-x-3"}, {"unit-x-4"},
	}}
	results, err := ig.InstanceId(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "foo"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: "x2 error"}},
			{Error: &params.Error{Message: "x3 error"}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (*instanceIdGetterSuite) TestInstanceIdError(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	ig := common.NewInstanceIdGetter(&fakeState{}, getCanRead)
	_, err := ig.InstanceId(params.Entities{[]params.Entity{{"unit-x-0"}}})
	c.Assert(err, gc.ErrorMatches, "pow")
}
