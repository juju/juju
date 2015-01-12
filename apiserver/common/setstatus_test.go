// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

type statusSetterSuite struct{}

var _ = gc.Suite(&statusSetterSuite{})

var _ state.StatusSetter = new(fakeStatusSetter)

type fakeStatusSetter struct {
	state.Entity
	status state.Status
	info   string
	data   map[string]interface{}
	err    error
	fetchError
}

func (s *fakeStatusSetter) SetStatus(status state.Status, info string, data map[string]interface{}) error {
	s.status = status
	s.info = info
	s.data = data
	return s.err
}

func (s *fakeStatusSetter) Status() (status state.Status, info string, data map[string]interface{}, err error) {
	return s.status, s.info, s.data, nil
}

func (s *fakeStatusSetter) UpdateStatus(data map[string]interface{}) error {
	for k, v := range data {
		s.data[k] = v
	}
	return s.err
}

func (*statusSetterSuite) TestSetStatus(c *gc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeStatusSetter{status: state.StatusAllocating, info: "blah", err: fmt.Errorf("x0 fails")},
			u("x/1"): &fakeStatusSetter{status: state.StatusInstalling, info: "blah"},
			u("x/2"): &fakeStatusSetter{status: state.StatusActive, info: "foo"},
			u("x/3"): &fakeStatusSetter{status: state.StatusError, info: "some info"},
			u("x/4"): &fakeStatusSetter{fetchError: "x3 error"},
			u("x/5"): &fakeStatusSetter{status: state.StatusStopping, info: "blah"},
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
	s := common.NewStatusSetter(st, getCanModify)
	args := params.SetStatus{
		Entities: []params.EntityStatus{
			{"unit-x-0", params.StatusInstalling, "bar", nil},
			{"unit-x-1", params.StatusActive, "bar", nil},
			{"unit-x-2", params.StatusStopping, "", nil},
			{"unit-x-3", params.StatusAllocating, "not really", nil},
			{"unit-x-4", params.StatusStopping, "", nil},
			{"unit-x-5", params.StatusError, "blarg", nil},
			{"unit-x-6", params.StatusActive, "42", nil},
			{"unit-x-7", params.StatusActive, "bar", nil},
		},
	}
	result, err := s.SetStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: "x0 fails"}},
			{nil},
			{nil},
			{nil},
			{&params.Error{Message: "x3 error"}},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})
	get := func(tag names.Tag) *fakeStatusSetter {
		return st.entities[tag].(*fakeStatusSetter)
	}
	c.Assert(get(u("x/1")).status, gc.Equals, state.StatusActive)
	c.Assert(get(u("x/1")).info, gc.Equals, "bar")
	c.Assert(get(u("x/2")).status, gc.Equals, state.StatusStopping)
	c.Assert(get(u("x/2")).info, gc.Equals, "")
	c.Assert(get(u("x/3")).status, gc.Equals, state.StatusAllocating)
	c.Assert(get(u("x/3")).info, gc.Equals, "not really")
	c.Assert(get(u("x/5")).status, gc.Equals, state.StatusError)
	c.Assert(get(u("x/5")).info, gc.Equals, "blarg")
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}

func (*statusSetterSuite) TestUpdateStatus(c *gc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			m("0"): &fakeStatusSetter{status: state.StatusAllocating, info: "blah", err: fmt.Errorf("x0 fails")},
			m("1"): &fakeStatusSetter{status: state.StatusError, info: "foo", data: map[string]interface{}{"foo": "blah"}},
			m("2"): &fakeStatusSetter{status: state.StatusError, info: "some info"},
			m("3"): &fakeStatusSetter{fetchError: "x3 error"},
			m("4"): &fakeStatusSetter{status: state.StatusActive},
			m("5"): &fakeStatusSetter{status: state.StatusStopping, info: ""},
		},
	}
	getCanModify := func() (common.AuthFunc, error) {
		x0 := m("0")
		x1 := m("1")
		x2 := m("2")
		x3 := m("3")
		x4 := m("4")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x1 || tag == x2 || tag == x3 || tag == x4
		}, nil
	}
	s := common.NewStatusSetter(st, getCanModify)
	args := params.SetStatus{
		Entities: []params.EntityStatus{
			{Tag: "machine-0", Data: nil},
			{Tag: "machine-1", Data: nil},
			{Tag: "machine-2", Data: map[string]interface{}{"foo": "bar"}},
			{Tag: "machine-3", Data: map[string]interface{}{"foo": "bar"}},
			{Tag: "machine-4", Data: map[string]interface{}{"foo": "bar"}},
			{Tag: "machine-5", Data: map[string]interface{}{"foo": "bar"}},
			{Tag: "machine-6", Data: nil},
		},
	}
	result, err := s.UpdateStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: "x0 fails"}},
			{nil},
			{nil},
			{&params.Error{Message: "x3 error"}},
			{&params.Error{Message: `"machine-4" is not in an error state`}},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})
	get := func(tag names.Tag) *fakeStatusSetter {
		return st.entities[tag].(*fakeStatusSetter)
	}
	c.Assert(get(m("1")).status, gc.Equals, state.StatusError)
	c.Assert(get(m("1")).info, gc.Equals, "foo")
	c.Assert(get(m("1")).data, gc.DeepEquals, map[string]interface{}{"foo": "blah"})
	c.Assert(get(m("2")).status, gc.Equals, state.StatusError)
	c.Assert(get(m("2")).info, gc.Equals, "some info")
	c.Assert(get(m("2")).data, gc.DeepEquals, map[string]interface{}{"foo": "bar"})
}
