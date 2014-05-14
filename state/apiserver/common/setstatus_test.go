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

type statusSetterSuite struct{}

var _ = gc.Suite(&statusSetterSuite{})

type fakeStatusSetter struct {
	state.Entity
	status params.Status
	info   string
	data   params.StatusData
	err    error
	fetchError
}

func (s *fakeStatusSetter) SetStatus(status params.Status, info string, data params.StatusData) error {
	s.status = status
	s.info = info
	s.data = data
	return s.err
}

func (s *fakeStatusSetter) Status() (status params.Status, info string, data params.StatusData, err error) {
	return s.status, s.info, s.data, nil
}

func (s *fakeStatusSetter) UpdateStatus(data params.StatusData) error {
	for k, v := range data {
		s.data[k] = v
	}
	return s.err
}

func (*statusSetterSuite) TestSetStatus(c *gc.C) {
	st := &fakeState{
		entities: map[string]entityWithError{
			"x0": &fakeStatusSetter{status: params.StatusPending, info: "blah", err: fmt.Errorf("x0 fails")},
			"x1": &fakeStatusSetter{status: params.StatusStarted, info: "foo"},
			"x2": &fakeStatusSetter{status: params.StatusError, info: "some info"},
			"x3": &fakeStatusSetter{fetchError: "x3 error"},
			"x4": &fakeStatusSetter{status: params.StatusStopped, info: ""},
		},
	}
	getCanModify := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			switch tag {
			case "x0", "x1", "x2", "x3":
				return true
			}
			return false
		}, nil
	}
	s := common.NewStatusSetter(st, getCanModify)
	args := params.SetStatus{
		Entities: []params.EntityStatus{
			{"x0", params.StatusStarted, "bar", nil},
			{"x1", params.StatusStopped, "", nil},
			{"x2", params.StatusPending, "not really", nil},
			{"x3", params.StatusStopped, "", nil},
			{"x4", params.StatusError, "blarg", nil},
			{"x5", params.StatusStarted, "42", nil},
		},
	}
	result, err := s.SetStatus(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: "x0 fails"}},
			{nil},
			{nil},
			{&params.Error{Message: "x3 error"}},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})
	get := func(tag string) *fakeStatusSetter {
		return st.entities[tag].(*fakeStatusSetter)
	}
	c.Assert(get("x1").status, gc.Equals, params.StatusStopped)
	c.Assert(get("x1").info, gc.Equals, "")
	c.Assert(get("x2").status, gc.Equals, params.StatusPending)
	c.Assert(get("x2").info, gc.Equals, "not really")
}

func (*statusSetterSuite) TestSetStatusError(c *gc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	s := common.NewStatusSetter(&fakeState{}, getCanModify)
	args := params.SetStatus{
		Entities: []params.EntityStatus{{"x0", "", "", nil}},
	}
	_, err := s.SetStatus(args)
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*statusSetterSuite) TestSetStatusNoArgsNoError(c *gc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	s := common.NewStatusSetter(&fakeState{}, getCanModify)
	result, err := s.SetStatus(params.SetStatus{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}

func (*statusSetterSuite) TestUpdateStatus(c *gc.C) {
	st := &fakeState{
		entities: map[string]entityWithError{
			"x0": &fakeStatusSetter{status: params.StatusPending, info: "blah", err: fmt.Errorf("x0 fails")},
			"x1": &fakeStatusSetter{status: params.StatusError, info: "foo", data: params.StatusData{"foo": "blah"}},
			"x2": &fakeStatusSetter{status: params.StatusError, info: "some info"},
			"x3": &fakeStatusSetter{fetchError: "x3 error"},
			"x4": &fakeStatusSetter{status: params.StatusStarted},
			"x5": &fakeStatusSetter{status: params.StatusStopped, info: ""},
		},
	}
	getCanModify := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			switch tag {
			case "x0", "x1", "x2", "x3", "x4":
				return true
			}
			return false
		}, nil
	}
	s := common.NewStatusSetter(st, getCanModify)
	args := params.SetStatus{
		Entities: []params.EntityStatus{
			{Tag: "x0", Data: nil},
			{Tag: "x1", Data: nil},
			{Tag: "x2", Data: params.StatusData{"foo": "bar"}},
			{Tag: "x3", Data: params.StatusData{"foo": "bar"}},
			{Tag: "x4", Data: params.StatusData{"foo": "bar"}},
			{Tag: "x5", Data: params.StatusData{"foo": "bar"}},
			{Tag: "x6", Data: nil},
		},
	}
	result, err := s.UpdateStatus(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: "x0 fails"}},
			{nil},
			{nil},
			{&params.Error{Message: "x3 error"}},
			{&params.Error{Message: `machine "x4" is not in an error state`}},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})
	get := func(tag string) *fakeStatusSetter {
		return st.entities[tag].(*fakeStatusSetter)
	}
	c.Assert(get("x1").status, gc.Equals, params.StatusError)
	c.Assert(get("x1").info, gc.Equals, "foo")
	c.Assert(get("x1").data, gc.DeepEquals, params.StatusData{"foo": "blah"})
	c.Assert(get("x2").status, gc.Equals, params.StatusError)
	c.Assert(get("x2").info, gc.Equals, "some info")
	c.Assert(get("x2").data, gc.DeepEquals, params.StatusData{"foo": "bar"})
}
