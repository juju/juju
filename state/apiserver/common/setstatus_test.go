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

type statusSetterSuite struct{}

var _ = gc.Suite(&statusSetterSuite{})

type fakeStatusSetterState struct {
	entities map[string]*fakeStatusSetter
}

func (st *fakeStatusSetterState) StatusSetter(tag string) (state.StatusSetter, error) {
	if statusSetter, ok := st.entities[tag]; ok {
		if statusSetter.err != nil {
			return nil, statusSetter.err
		}
		return statusSetter, nil
	}
	return nil, errors.NotFoundf("entity %q", tag)
}

type fakeStatusSetter struct {
	tag    string
	status params.Status
	info   string
	err    error
}

func (s *fakeStatusSetter) SetStatus(status params.Status, info string) error {
	s.status = status
	s.info = info
	return s.err
}

func (*statusSetterSuite) TestSetStatus(c *gc.C) {
	st := &fakeStatusSetterState{
		entities: map[string]*fakeStatusSetter{
			"x0": {status: params.StatusPending, info: "blah", err: fmt.Errorf("x0 fails")},
			"x1": {status: params.StatusStarted, info: "foo"},
			"x2": {status: params.StatusError, info: "some info"},
			"x3": {status: params.StatusStopped, info: ""},
		},
	}
	getCanModify := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			switch tag {
			case "x0", "x1", "x2":
				return true
			}
			return false
		}, nil
	}
	s := common.NewStatusSetter(st, getCanModify)
	args := params.SetStatus{
		Entities: []params.SetEntityStatus{
			{"x0", params.StatusStarted, "bar"},
			{"x1", params.StatusStopped, ""},
			{"x2", params.StatusPending, "not really"},
			{"x3", params.StatusError, "blarg"},
			{"x4", params.StatusStarted, "42"},
		},
	}
	result, err := s.SetStatus(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: "x0 fails"}},
			{nil},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})
	c.Assert(st.entities["x1"].status, gc.Equals, params.StatusStopped)
	c.Assert(st.entities["x1"].info, gc.Equals, "")
	c.Assert(st.entities["x2"].status, gc.Equals, params.StatusPending)
	c.Assert(st.entities["x2"].info, gc.Equals, "not really")

	// Test compatibility with v1.12.
	// Remove the rest of this test once it's deprecated.
	// DEPRECATE(v1.14)
	args.Machines = args.Entities
	args.Entities = nil
	result, err = s.SetStatus(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: "x0 fails"}},
			{nil},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})
}

func (*statusSetterSuite) TestSetStatusError(c *gc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	s := common.NewStatusSetter(&fakeStatusSetterState{}, getCanModify)
	args := params.SetStatus{
		Entities: []params.SetEntityStatus{{"x0", "", ""}},
	}
	_, err := s.SetStatus(args)
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*statusSetterSuite) TestSetStatusNoArgsNoError(c *gc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	s := common.NewStatusSetter(&fakeStatusSetterState{}, getCanModify)
	result, err := s.SetStatus(params.SetStatus{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
