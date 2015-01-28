// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

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

type agentStatusSetterSuite struct{}

var _ = gc.Suite(&agentStatusSetterSuite{})

type fakeAgentStatusSetter struct {
	state.Unit
	status state.Status
	info   string
	data   map[string]interface{}
	err    error
	fetchError
}

func (s *fakeAgentStatusSetter) Agent() state.StatusSetterGetter {
	return s
}

func (s *fakeAgentStatusSetter) SetStatus(status state.Status, info string, data map[string]interface{}) error {
	s.status = status
	s.info = info
	s.data = data
	return s.err
}

func (*agentStatusSetterSuite) TestSetUnitAgentStatus(c *gc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeAgentStatusSetter{status: state.StatusAllocating, info: "blah", err: fmt.Errorf("x0 fails")},
			u("x/1"): &fakeAgentStatusSetter{status: state.StatusInstalling, info: "blah"},
			u("x/2"): &fakeAgentStatusSetter{status: state.StatusActive, info: "foo"},
			u("x/3"): &fakeAgentStatusSetter{status: state.StatusError, info: "some info"},
			u("x/4"): &fakeAgentStatusSetter{fetchError: "x3 error"},
			u("x/5"): &fakeAgentStatusSetter{status: state.StatusStopping, info: "blah"},
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
	s := common.NewAgentStatusSetter(st, getCanModify)
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
	get := func(tag names.Tag) *fakeAgentStatusSetter {
		return st.entities[tag].(*fakeAgentStatusSetter)
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
