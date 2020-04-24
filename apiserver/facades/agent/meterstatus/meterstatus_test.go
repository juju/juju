// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/meterstatus"
	"github.com/juju/juju/apiserver/facades/agent/meterstatus/mocks"
	meterstatustesting "github.com/juju/juju/apiserver/facades/agent/meterstatus/testing"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	jujufactory "github.com/juju/juju/testing/factory"
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

func (s *meterStatusSuite) TestGetMeterStatusUnauthenticated(c *gc.C) {
	application, err := s.unit.Application()
	c.Assert(err, jc.ErrorIsNil)
	otherunit := s.Factory.MakeUnit(c, &jujufactory.UnitParams{Application: application})
	args := params.Entities{Entities: []params.Entity{{otherunit.Tag().String()}}}
	result, err := s.status.GetMeterStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "permission denied")
	c.Assert(result.Results[0].Code, gc.Equals, "")
	c.Assert(result.Results[0].Info, gc.Equals, "")
}

func (s *meterStatusSuite) TestGetMeterStatusBadTag(c *gc.C) {
	tags := []string{
		"user-admin",
		"unit-nosuchunit",
		"thisisnotatag",
		"machine-0",
		"model-blah",
	}
	args := params.Entities{Entities: make([]params.Entity, len(tags))}
	for i, tag := range tags {
		args.Entities[i] = params.Entity{Tag: tag}
	}
	result, err := s.status.GetMeterStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(tags))
	for i, result := range result.Results {
		c.Logf("checking result %d", i)
		c.Assert(result.Code, gc.Equals, "")
		c.Assert(result.Info, gc.Equals, "")
		c.Assert(result.Error, gc.ErrorMatches, "permission denied")
	}
}

func (s *meterStatusSuite) TestGetMeterStatus(c *gc.C) {
	meterstatustesting.TestGetMeterStatus(c, s.status, s.unit)
}

func (s *meterStatusSuite) TestWatchMeterStatus(c *gc.C) {
	status, ctrl := s.setupMeterStatusAPI(c, func(mocks meterStatusAPIMocks) {
		aExp := mocks.authorizer.EXPECT()
		sExp := mocks.state.EXPECT()
		rExp := mocks.resources.EXPECT()

		tag := s.unit.UnitTag()
		aExp.GetAuthTag().Return(tag)

		aExp.AuthOwner(tag).Return(true)
		sExp.Unit(tag.Id()).Return(s.unit, nil)
		rExp.Register(gomock.Any()).Return("1")

		aExp.AuthOwner(names.NewUnitTag("foo/42")).Return(false)
	})
	defer ctrl.Finish()

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.unit.UnitTag().String()},
		{Tag: "unit-foo-42"},
	}}
	result, err := status.WatchMeterStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *meterStatusSuite) TestWatchMeterStatusWithStateChange(c *gc.C) {
	status, ctrl := s.setupMeterStatusAPI(c, func(mocks meterStatusAPIMocks) {
		aExp := mocks.authorizer.EXPECT()
		sExp := mocks.state.EXPECT()
		rExp := mocks.resources.EXPECT()

		tag := s.unit.UnitTag()
		aExp.GetAuthTag().Return(tag)

		aExp.AuthOwner(tag).Return(true)
		sExp.Unit(tag.Id()).Return(s.unit, nil)
		rExp.Register(gomock.Any()).Return("1")
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

func (s *meterStatusSuite) TestWatchMeterStatusWithApplicationTag(c *gc.C) {
	app, err := s.unit.Application()
	c.Assert(err, jc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)

	unit := units[0]

	status, ctrl := s.setupMeterStatusAPI(c, func(mocks meterStatusAPIMocks) {
		aExp := mocks.authorizer.EXPECT()
		sExp := mocks.state.EXPECT()
		rExp := mocks.resources.EXPECT()

		tag := names.ApplicationTag{
			Name: "mysql",
		}
		aExp.GetAuthTag().Return(tag)

		sExp.Application(tag.Name).Return(app, nil)
		sExp.Unit(unit.Tag().Id()).Return(unit, nil)
		rExp.Register(gomock.Any()).Return("1")
	})
	defer ctrl.Finish()

	args := params.Entities{Entities: []params.Entity{
		{Tag: unit.UnitTag().String()},
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
