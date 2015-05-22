// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
)

type statusSetterSuite struct{}

var _ = gc.Suite(&statusSetterSuite{})

var _ state.StatusSetter = new(fakeStatus)

type fakeStatus struct {
	state.Entity
	status  state.Status
	info    string
	data    map[string]interface{}
	updated time.Time
	err     error
	fetchError
}

func (s *fakeStatus) SetStatus(status state.Status, info string, data map[string]interface{}) error {
	s.status = status
	s.info = info
	s.data = data
	s.updated = time.Now()
	return s.err
}

func (s *fakeStatus) Status() (state.StatusInfo, error) {
	return state.StatusInfo{
		s.status, s.info, s.data, &s.updated,
	}, s.err
}

func (s *fakeStatus) UpdateStatus(data map[string]interface{}) error {
	for k, v := range data {
		s.data[k] = v
	}
	return s.err
}

func (*statusSetterSuite) TestSetServiceStatus(c *gc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeService{
				tag: serviceTag("x/0"),
				serviceStatus: state.StatusInfo{
					Status:  state.StatusAllocating,
					Message: "blah",
				},
				err: fmt.Errorf("x0 fails"),
			},
			u("x/1"): &fakeService{
				tag: serviceTag("x/1"),
				serviceStatus: state.StatusInfo{
					Status:  state.StatusInstalling,
					Message: "blah",
				},
			},
			u("x/2"): &fakeService{
				tag: serviceTag("x/2"),
				serviceStatus: state.StatusInfo{
					Status:  state.StatusActive,
					Message: "foo",
				},
			},
			u("x/3"): &fakeService{
				tag: serviceTag("x/3"),
				serviceStatus: state.StatusInfo{
					Status:  state.StatusError,
					Message: "some info",
				},
			},
			u("x/4"): &fakeService{
				tag:           serviceTag("x/4"),
				serviceStatus: state.StatusInfo{},
				fetchError:    "x3 error",
			},
			u("x/5"): &fakeService{
				tag: serviceTag("x/5"),
				serviceStatus: state.StatusInfo{
					Status:  state.StatusStopping,
					Message: "blah",
				},
			},
		},
	}
	getCanModify := func() (common.AuthFunc, error) {
		x0 := serviceTag("x/0")
		x1 := serviceTag("x/1")
		x2 := serviceTag("x/2")
		x3 := serviceTag("x/3")
		x4 := serviceTag("x/4")
		x5 := serviceTag("x/5")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x1 || tag == x2 || tag == x3 || tag == x4 || tag == x5
		}, nil
	}
	s := common.NewServiceStatusSetter(st, getCanModify)
	args := params.SetStatus{
		Entities: []params.EntityStatus{
			{"unit-x-0", params.StatusInstalling, "bar", nil},
			{"unit-x-1", params.StatusActive, "bar", nil},
			{"unit-x-2", params.StatusStopping, "", nil},
			{"unit-x-3", params.StatusAllocating, "not really", nil},
			{"unit-x-4", params.StatusStopping, "", nil},
			{"unit-x-5", params.StatusError, "blarg", nil},
		},
	}
	result, err := common.ServiceSetStatus(s, args, fakeServiceFromUnitTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: "x0 fails"}},
			{nil},
			{nil},
			{nil},
			{&params.Error{Message: "x3 error"}},
			{nil},
		},
	})
	get := func(tag names.Tag) *fakeService {
		return st.entities[tag].(*fakeService)
	}
	c.Assert(get(u("x/1")).serviceStatus.Status, gc.Equals, state.StatusActive)
	c.Assert(get(u("x/1")).serviceStatus.Message, gc.Equals, "bar")
	c.Assert(get(u("x/2")).serviceStatus.Status, gc.Equals, state.StatusStopping)
	c.Assert(get(u("x/2")).serviceStatus.Message, gc.Equals, "")
	c.Assert(get(u("x/3")).serviceStatus.Status, gc.Equals, state.StatusAllocating)
	c.Assert(get(u("x/3")).serviceStatus.Message, gc.Equals, "not really")
	c.Assert(get(u("x/5")).serviceStatus.Status, gc.Equals, state.StatusError)
	c.Assert(get(u("x/5")).serviceStatus.Message, gc.Equals, "blarg")

}

func (*statusSetterSuite) TestSetStatus(c *gc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeStatus{status: state.StatusAllocating, info: "blah", err: fmt.Errorf("x0 fails")},
			u("x/1"): &fakeStatus{status: state.StatusInstalling, info: "blah"},
			u("x/2"): &fakeStatus{status: state.StatusActive, info: "foo"},
			u("x/3"): &fakeStatus{status: state.StatusError, info: "some info"},
			u("x/4"): &fakeStatus{fetchError: "x3 error"},
			u("x/5"): &fakeStatus{status: state.StatusStopping, info: "blah"},
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
	get := func(tag names.Tag) *fakeStatus {
		return st.entities[tag].(*fakeStatus)
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
			m("0"): &fakeStatus{status: state.StatusAllocating, info: "blah", err: fmt.Errorf("x0 fails")},
			m("1"): &fakeStatus{status: state.StatusError, info: "foo", data: map[string]interface{}{"foo": "blah"}},
			m("2"): &fakeStatus{status: state.StatusError, info: "some info"},
			m("3"): &fakeStatus{fetchError: "x3 error"},
			m("4"): &fakeStatus{status: state.StatusActive},
			m("5"): &fakeStatus{status: state.StatusStopping, info: ""},
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
			{&params.Error{Message: "machine 4 is not in an error state"}},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})
	get := func(tag names.Tag) *fakeStatus {
		return st.entities[tag].(*fakeStatus)
	}
	c.Assert(get(m("1")).status, gc.Equals, state.StatusError)
	c.Assert(get(m("1")).info, gc.Equals, "foo")
	c.Assert(get(m("1")).data, gc.DeepEquals, map[string]interface{}{"foo": "blah"})
	c.Assert(get(m("2")).status, gc.Equals, state.StatusError)
	c.Assert(get(m("2")).info, gc.Equals, "some info")
	c.Assert(get(m("2")).data, gc.DeepEquals, map[string]interface{}{"foo": "bar"})
}
