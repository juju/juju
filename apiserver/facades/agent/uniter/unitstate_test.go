// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/unitstate"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type unitStateSuite struct {
	testing.BaseSuite

	unitTag1 names.UnitTag
	uniter   *UniterAPI

	controllerConfigService *mocks.MockControllerConfigService
	unitStateService        *MockUnitStateService
}

func TestUnitStateSuite(t *stdtesting.T) {
	tc.Run(t, &unitStateSuite{})
}

func (s *unitStateSuite) SetUpTest(c *tc.C) {
	s.unitTag1 = names.NewUnitTag("wordpress/0")
	c.Cleanup(func() {
		s.unitTag1 = names.UnitTag{}
	})
}

func (s *unitStateSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = mocks.NewMockControllerConfigService(ctrl)
	s.unitStateService = NewMockUnitStateService(ctrl)

	unitAuthFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag.Id() == s.unitTag1.Id() {
				return true
			}
			return false
		}, nil
	}

	s.uniter = &UniterAPI{
		controllerConfigService: s.controllerConfigService,
		unitStateService:        s.unitStateService,
		accessUnit:              unitAuthFunc,
		logger:                  loggertesting.WrapCheckLog(c),
	}

	c.Cleanup(func() {
		s.controllerConfigService = nil
		s.unitStateService = nil
		s.uniter = nil
	})
	return ctrl
}

func (s *unitStateSuite) expectGetState(c *tc.C, name string) (map[string]string, string, map[int]string, string, string) {
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
	expSecretState := "secret testing"

	unitName := unittesting.GenNewName(c, name)

	s.unitStateService.EXPECT().GetState(gomock.Any(), unitName).Return(unitstate.RetrievedUnitState{
		CharmState:    expCharmState,
		UniterState:   expUniterState,
		RelationState: expRelationState,
		StorageState:  expStorageState,
		SecretState:   expSecretState,
	}, nil)

	return expCharmState, expUniterState, expRelationState, expStorageState, expSecretState
}

func (s *unitStateSuite) TestState(c *tc.C) {
	defer s.setupMocks(c).Finish()
	expCharmState, expUniterState, expRelationState, expStorageState, expSecretState := s.expectGetState(c, "wordpress/0")

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "not-a-unit-tag"},
			{Tag: "unit-wordpress-0"},
			{Tag: "unit-mysql-0"}, // not accessible by current user
			{Tag: "unit-notfound-0"},
		},
	}
	result, err := s.uniter.State(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.UnitStateResults{
		Results: []params.UnitStateResult{
			{Error: &params.Error{Message: `"not-a-unit-tag" is not a valid tag`}},
			{
				Error:         nil,
				CharmState:    expCharmState,
				UniterState:   expUniterState,
				RelationState: expRelationState,
				StorageState:  expStorageState,
				SecretState:   expSecretState,
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *unitStateSuite) TestSetStateUniterState(c *tc.C) {
	defer s.setupMocks(c).Finish()
	expUniterState := "testing"

	args := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{
			{Tag: "not-a-unit-tag", UniterState: &expUniterState},
			{Tag: "unit-wordpress-0", UniterState: &expUniterState},
			{Tag: "unit-mysql-0", UniterState: &expUniterState}, // not accessible by current user
			{Tag: "unit-notfound-0", UniterState: &expUniterState},
		},
	}

	expectedState := unitstate.UnitState{
		Name:        "wordpress/0",
		UniterState: &expUniterState,
	}
	s.unitStateService.EXPECT().SetState(gomock.Any(), expectedState).Return(nil)

	result, err := s.uniter.SetState(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: `"not-a-unit-tag" is not a valid tag`}},
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}
