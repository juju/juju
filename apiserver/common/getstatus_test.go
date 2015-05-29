// Copyright 2015 Canonical Ltd.
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

type statusGetterSuite struct{}

var _ = gc.Suite(&statusGetterSuite{})

var _ state.StatusGetter = new(fakeStatus)

func fakeServiceFromUnitTag(st state.EntityFinder, unitTag string) (common.StatusService, error) {
	tag, err := names.ParseUnitTag(unitTag)
	if err != nil {
		return nil, err
	}
	entity, err := st.FindEntity(tag)
	service, ok := entity.(*fakeService)
	if !ok {
		return nil, err
	}
	return service, nil
}

func fakeIsLeaderCheck(_ state.EntityFinder, _ string) (bool, error) {
	return true, nil
}

type fakeService struct {
	tag         names.Tag
	status      state.StatusInfo
	statusError error

	unitsStatus      map[string]state.StatusInfo
	unitsStatusError error
	serviceStatus    state.StatusInfo
	err              error
	fetchError
}

func (s *fakeService) Status() (state.StatusInfo, error) {
	return s.status, s.statusError
}

func (s *fakeService) UnitsStatus() (map[string]state.StatusInfo, error) {
	return s.unitsStatus, s.unitsStatusError
}

func (s *fakeService) Tag() names.Tag {
	return s.tag
}

func (s *fakeService) SetStatus(status state.Status, info string, data map[string]interface{}) error {
	s.serviceStatus.Status = status
	s.serviceStatus.Message = info
	s.serviceStatus.Data = data
	updated := time.Now()
	s.serviceStatus.Since = &updated
	return s.err
}

func (*statusGetterSuite) TestServiceStatus(c *gc.C) {
	now := time.Now()
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/1"): &fakeService{
				tag: serviceTag("wordpress-1"),
				status: state.StatusInfo{
					Status:  state.StatusActive,
					Message: "foo service",
					Since:   &now,
				},
				statusError: nil,
				unitsStatus: map[string]state.StatusInfo{
					"unit-x-1": state.StatusInfo{
						Status:  state.StatusActive,
						Message: "foo",
						Since:   &now,
					},
					"unit-x-2": state.StatusInfo{
						Status:  state.StatusActive,
						Message: "foo 2",
						Since:   &now,
					},
				},
				unitsStatusError: nil,
			},
		},
	}

	expected := params.ServiceStatusResults{
		Results: []params.ServiceStatusResult{
			params.ServiceStatusResult{
				Service: params.StatusResult{
					Error:  nil,
					Id:     "",
					Life:   "",
					Status: "active",
					Info:   "foo service",
					Data:   nil,
					Since:  &now,
				},
				Units: map[string]params.StatusResult{
					"unit-x-1": params.StatusResult{
						Error:  nil,
						Id:     "",
						Life:   "",
						Status: "active",
						Info:   "foo",
						Data:   nil,
						Since:  &now,
					},
					"unit-x-2": params.StatusResult{
						Error:  nil,
						Id:     "",
						Life:   "",
						Status: "active",
						Info:   "foo 2",
						Data:   nil,
						Since:  &now,
					},
				},
				Error: nil},
		},
	}
	getCanAccess := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool { return true }, nil
	}

	args := params.Entities{
		Entities: []params.Entity{
			params.Entity{
				Tag: "unit-x-1",
			},
		},
	}

	sg := common.NewServiceStatusGetter(st, getCanAccess)
	result, err := common.ServiceStatus(sg, args, fakeServiceFromUnitTag, fakeIsLeaderCheck)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expected)

	leaderCheck := func(_ state.EntityFinder, tag string) (bool, error) {
		return false, nil
	}

	expected = params.ServiceStatusResults{
		Results: []params.ServiceStatusResult{
			{
				Service: params.StatusResult{
					Error:  nil,
					Id:     "",
					Life:   "",
					Status: "",
					Info:   "",
					Data:   nil,
					Since:  nil},
				Units: map[string]params.StatusResult(nil),
				Error: &params.Error{"this unit is not the leader", ""},
			},
		},
	}
	result, err = common.ServiceStatus(sg, args, fakeServiceFromUnitTag, leaderCheck)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expected)

}

func (*statusGetterSuite) TestStatus(c *gc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeStatus{status: state.StatusAllocating, info: "blah", err: fmt.Errorf("x0 fails")},
			u("x/1"): &fakeStatus{status: state.StatusInstalling, info: "blah"},
			u("x/2"): &fakeStatus{status: state.StatusActive, info: "foo"},
			u("x/3"): &fakeStatus{status: state.StatusError, info: "some info", data: map[string]interface{}{"foo": "bar"}},
			u("x/4"): &fakeStatus{fetchError: "x3 error"},
			u("x/5"): &fakeStatus{status: state.StatusStopping, info: "blah"},
		},
	}
	getCanAccess := func() (common.AuthFunc, error) {
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
	s := common.NewStatusGetter(st, getCanAccess)
	args := params.Entities{
		Entities: []params.Entity{
			{"unit-x-0"},
			{"unit-x-1"},
			{"unit-x-2"},
			{"unit-x-3"},
			{"unit-x-4"},
			{"unit-x-5"},
			{"unit-x-6"},
			{"unit-x-7"},
			{"machine-1"},
			{"invalid"},
		},
	}
	result, err := s.Status(args)
	c.Assert(err, jc.ErrorIsNil)
	// Zero out the updated timestamps so we can easily check the results.
	for i, statusResult := range result.Results {
		r := statusResult
		if r.Status != "" {
			c.Assert(r.Since, gc.NotNil)
		}
		r.Since = nil
		result.Results[i] = r
	}
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Status: "allocating", Info: "blah", Error: &params.Error{Message: "x0 fails"}},
			{Status: "installing", Info: "blah"},
			{Status: "active", Info: "foo"},
			{Status: "error", Info: "some info", Data: map[string]interface{}{"foo": "bar"}},
			{Error: &params.Error{Message: "x3 error"}},
			{Status: "stopping", Info: "blah"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ServerError(`"invalid" is not a valid tag`)},
		},
	})
}

func (*statusGetterSuite) TestStatusError(c *gc.C) {
	getCanAccess := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	s := common.NewStatusGetter(&fakeState{}, getCanAccess)
	args := params.Entities{
		Entities: []params.Entity{{"x0"}},
	}
	_, err := s.Status(args)
	c.Assert(err, gc.ErrorMatches, "pow")
}
