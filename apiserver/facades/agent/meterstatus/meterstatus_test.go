// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/meterstatus"
	"github.com/juju/juju/apiserver/facades/agent/meterstatus/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(&meterStatusSuite{})

type meterStatusSuite struct {
	jujutesting.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	unit *state.Unit

	status meterstatus.MeterStatus
}

func (s *meterStatusSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.unit = s.Factory.MakeUnit(c, nil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.unit.UnitTag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	status, err := meterstatus.NewMeterStatusAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.status = status
}

func (s *meterStatusSuite) TestGetMeterStatusNoOp(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{{Tag: s.unit.Tag().String()}}}
	result, err := s.status.GetMeterStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Code, gc.Equals, "")
	c.Assert(result.Results[0].Info, gc.Equals, "")
}

func (s *meterStatusSuite) TestWatchMeterStatus(c *gc.C) {
	status, ctrl := s.setupMeterStatusAPI(c, func(mocks meterStatusAPIMocks) {
		rExp := mocks.resources.EXPECT()

		rExp.Register(gomock.Any()).Return("1")
		rExp.Register(gomock.Any()).Return("2")
	})
	defer ctrl.Finish()

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.unit.UnitTag().String()},
	}}
	result, err := status.WatchMeterStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
		},
	})
}

type meterStatusAPIMocks struct {
	state      *mocks.MockMeterStatusState
	resources  *facademocks.MockResources
	authorizer *facademocks.MockAuthorizer
}

func (s *meterStatusSuite) setupMeterStatusAPI(c *gc.C, fn func(meterStatusAPIMocks)) (*meterstatus.MeterStatusAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	mockState := mocks.NewMockMeterStatusState(ctrl)
	mockResources := facademocks.NewMockResources(ctrl)
	mockAuthorizer := facademocks.NewMockAuthorizer(ctrl)

	mockAuthorizer.EXPECT().AuthUnitAgent().Return(true)

	status, err := meterstatus.NewMeterStatusAPI(mockState, mockResources, mockAuthorizer)
	c.Assert(err, jc.ErrorIsNil)

	fn(meterStatusAPIMocks{
		state:      mockState,
		resources:  mockResources,
		authorizer: mockAuthorizer,
	})

	return status, ctrl
}
