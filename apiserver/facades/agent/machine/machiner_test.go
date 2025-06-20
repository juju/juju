// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/agent/machine"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type machinerSuite struct {
	commonSuite

	machiner           *machine.MachinerAPI
	networkService     *MockNetworkService
	machineService     *MockMachineService
	applicationService *MockApplicationService
	watcherRegistry    *MockWatcherRegistry
}

func TestMachinerSuite(t *testing.T) {
	tc.Run(t, &machinerSuite{})
}

func (s *machinerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = NewMockWatcherRegistry(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	return ctrl
}

func (s *machinerSuite) makeAPI(c *tc.C) {
	st := s.ControllerModel(c).State()
	// Create a machiner API for machine 1.
	machiner, err := machine.NewMachinerAPIForState(
		c.Context(),
		st,
		clock.WallClock,
		s.ControllerDomainServices(c).ControllerConfig(),
		s.ControllerDomainServices(c).ControllerNode(),
		s.ControllerDomainServices(c).ModelInfo(),
		s.networkService,
		s.applicationService,
		s.machineService,
		s.watcherRegistry,
		s.authorizer,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	s.machiner = machiner
}

func (s *machinerSuite) TestMachinerFailsWithNonMachineAgentUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("ubuntu/1")
	st := s.ControllerModel(c).State()
	aMachiner, err := machine.NewMachinerAPIForState(
		c.Context(),
		st,
		clock.WallClock,
		s.ControllerDomainServices(c).ControllerConfig(),
		nil,
		nil,
		s.networkService,
		s.applicationService,
		s.machineService,
		s.watcherRegistry,
		anAuthorizer,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.NotNil)
	c.Assert(aMachiner, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *machinerSuite) TestSetStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	now := time.Now()

	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Since:   &now,
	}
	err := s.machine0.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Stopped,
		Message: "foo",
		Since:   &now,
	}
	err = s.machine1.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-1", Status: status.Error.String(), Info: "not really"},
			{Tag: "machine-0", Status: status.Stopped.String(), Info: "foobar"},
			{Tag: "machine-42", Status: status.Started.String(), Info: "blah"},
		}}
	result, err := s.machiner.SetStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify machine 0 - no change.
	statusInfo, err := s.machine0.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Started)
	c.Assert(statusInfo.Message, tc.Equals, "blah")
	// ...machine 1 is fine though.
	statusInfo, err = s.machine1.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Error)
	c.Assert(statusInfo.Message, tc.Equals, "not really")
}

func (s *machinerSuite) TestLife(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	exp := s.machineService.EXPECT()
	exp.GetMachineLife(gomock.Any(), coremachine.Name("1")).Return(life.Dead, nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.machiner.Life(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *machinerSuite) TestEnsureDead(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	c.Assert(s.machine0.Life(), tc.Equals, state.Alive)
	c.Assert(s.machine1.Life(), tc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	s.machineService.EXPECT().EnsureDeadMachine(gomock.Any(), coremachine.Name("1")).Return(nil).Times(2)
	result, err := s.machiner.EnsureDead(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine0.Life(), tc.Equals, state.Alive)
	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.Life(), tc.Equals, state.Dead)

	// Try it again on a Dead machine; should work.
	args = params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	}
	result, err = s.machiner.EnsureDead(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	// Verify Life is unchanged.
	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.Life(), tc.Equals, state.Dead)
}

func (s *machinerSuite) TestWatch(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.machiner.Watch(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *machinerSuite) TestRecordAgentStartInformation(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("1")).Return("uuid-1", nil)
	s.machineService.EXPECT().SetMachineHostname(gomock.Any(), coremachine.UUID("uuid-1"), "thundering-herds").Return(nil)

	args := params.RecordAgentStartInformationArgs{Args: []params.RecordAgentStartInformationArg{
		{Tag: "machine-1", Hostname: "thundering-herds"},
		{Tag: "machine-0", Hostname: "eldritch-octopii"},
		{Tag: "machine-42", Hostname: "missing-gem"},
	}}

	result, err := s.machiner.RecordAgentStartInformation(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *machinerSuite) TestSetObservedNetworkConfig(c *tc.C) {
	c.Skip(`This suite is missing tests for SetObservedNetworkConfig.
Those tests will be added once the call to the common network API is gone.
`)
}
