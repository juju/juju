// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
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
		return "", fmt.Errorf(f.err)
	}
	return instance.Id(f.instanceId), nil
}

func (*instanceIdGetterSuite) TestInstanceId(c *gc.C) {
	st := &fakeState{
		entities: map[string]entityWithError{
			"x0": &fakeInstanceIdGetter{instanceId: "foo"},
			"x1": &fakeInstanceIdGetter{instanceId: "bar"},
			"x2": &fakeInstanceIdGetter{instanceId: "baz", err: "x2 error"},
			"x3": &fakeInstanceIdGetter{fetchError: "x3 error"},
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
	ig := common.NewInstanceIdGetter(st, getCanRead)
	entities := params.Entities{[]params.Entity{
		{"x0"}, {"x1"}, {"x2"}, {"x3"}, {"x4"},
	}}
	results, err := ig.InstanceId(entities)
	c.Assert(err, gc.IsNil)
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
	_, err := ig.InstanceId(params.Entities{[]params.Entity{{"x0"}}})
	c.Assert(err, gc.ErrorMatches, "pow")
}
