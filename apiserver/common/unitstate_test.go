// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type unitStateSuite struct {
	testing.BaseSuite

	unitTag1 names.UnitTag

	api         *common.UnitStateAPI
	mockBackend *mocks.MockUnitStateBackend
	mockUnit    *mocks.MockUnitStateUnit
	mockOp      *mocks.MockModelOperation
}

var _ = gc.Suite(&unitStateSuite{})

func (s *unitStateSuite) SetUpTest(c *gc.C) {
	s.unitTag1 = names.NewUnitTag("wordpress/0")
}

func (s *unitStateSuite) assertBackendApi(c *gc.C) *gomock.Controller {
	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: s.unitTag1,
	}

	ctrl := gomock.NewController(c)
	s.mockBackend = mocks.NewMockUnitStateBackend(ctrl)
	s.mockUnit = mocks.NewMockUnitStateUnit(ctrl)
	s.mockOp = mocks.NewMockModelOperation(ctrl)

	unitAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag.Id() == s.unitTag1.Id() {
				return true
			}
			return false
		}, nil
	}

	s.api = common.NewUnitStateAPI(
		s.mockBackend, resources, authorizer, unitAuthFunc, loggo.GetLogger("juju.apiserver.common"))
	return ctrl
}

func (s *unitStateSuite) expectState() (map[string]string, string, map[int]string, string) {
	expCharmState := map[string]string{
		"foo.bar":  "baz",
		"payload$": "enc0d3d",
	}
	expUniterState := "testing"
	expRelationState := map[int]string{
		1: "one",
		2: "two",
	}
	expStorageState := "storage testing"

	unitState := state.NewUnitState()
	unitState.SetCharmState(expCharmState)
	unitState.SetUniterState(expUniterState)
	unitState.SetRelationState(expRelationState)
	unitState.SetStorageState(expStorageState)

	exp := s.mockUnit.EXPECT()
	exp.State().Return(unitState, nil)

	return expCharmState, expUniterState, expRelationState, expStorageState
}

func (s *unitStateSuite) expectUnit() {
	exp := s.mockBackend.EXPECT()
	exp.Unit(s.unitTag1.Id()).Return(s.mockUnit, nil)
}

func (s *unitStateSuite) expectSetStateOperation() string {
	unitState := state.NewUnitState()
	expUniterState := "testing"
	unitState.SetUniterState(expUniterState)

	// Mock controller config which provides the limits passed to SetStateOperation.
	s.mockBackend.EXPECT().ControllerConfig().Return(
		controller.Config{
			"max-charm-state-size": 123,
			"max-agent-state-size": 456,
		}, nil)

	exp := s.mockUnit.EXPECT()
	exp.SetStateOperation(
		unitState,
		state.UnitStateSizeLimits{
			MaxCharmStateSize: 123,
			MaxAgentStateSize: 456,
		},
	).Return(s.mockOp)
	return expUniterState
}

func (s *unitStateSuite) expectApplyOperation() {
	exp := s.mockBackend.EXPECT()
	exp.ApplyOperation(s.mockOp).Return(nil)
}

func (s *unitStateSuite) TestState(c *gc.C) {
	defer s.assertBackendApi(c).Finish()
	s.expectUnit()
	expCharmState, expUniterState, expRelationState, expStorageState := s.expectState()

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "not-a-unit-tag"},
			{Tag: "unit-wordpress-0"},
			{Tag: "unit-mysql-0"}, // not accessible by current user
			{Tag: "unit-notfound-0"},
		},
	}
	result, err := s.api.State(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.UnitStateResults{
		Results: []params.UnitStateResult{
			{Error: &params.Error{Message: `"not-a-unit-tag" is not a valid tag`}},
			{
				Error:         nil,
				CharmState:    expCharmState,
				UniterState:   expUniterState,
				RelationState: expRelationState,
				StorageState:  expStorageState,
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *unitStateSuite) TestSetStateUniterState(c *gc.C) {
	defer s.assertBackendApi(c).Finish()
	s.expectUnit()
	expUniterState := s.expectSetStateOperation()
	s.expectApplyOperation()

	args := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{
			{Tag: "not-a-unit-tag", UniterState: &expUniterState},
			{Tag: "unit-wordpress-0", UniterState: &expUniterState},
			{Tag: "unit-mysql-0", UniterState: &expUniterState}, // not accessible by current user
			{Tag: "unit-notfound-0", UniterState: &expUniterState},
		},
	}

	result, err := s.api.SetState(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: `"not-a-unit-tag" is not a valid tag`}},
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}
