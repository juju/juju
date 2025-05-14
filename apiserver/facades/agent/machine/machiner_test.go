// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/machine"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type machinerSuite struct {
	commonSuite

	machiner        *machine.MachinerAPI
	networkService  *MockNetworkService
	machineService  *MockMachineService
	watcherRegistry *MockWatcherRegistry
}

var _ = tc.Suite(&machinerSuite{})

func (s *machinerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = NewMockWatcherRegistry(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	return ctrl
}

func (s *machinerSuite) makeAPI(c *tc.C) {
	st := s.ControllerModel(c).State()
	// Create a machiner API for machine 1.
	machiner, err := machine.NewMachinerAPIForState(
		c.Context(),
		st,
		st,
		clock.WallClock,
		s.ControllerDomainServices(c).ControllerConfig(),
		s.ControllerDomainServices(c).ModelInfo(),
		s.networkService,
		s.machineService,
		s.watcherRegistry,
		common.NewResources(),
		s.authorizer,
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
		st,
		clock.WallClock,
		s.ControllerDomainServices(c).ControllerConfig(),
		nil,
		s.networkService,
		s.machineService,
		s.watcherRegistry,
		common.NewResources(),
		anAuthorizer,
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

	err := s.machine1.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.Life(), tc.Equals, state.Dead)

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

func (s *machinerSuite) TestSetMachineAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	c.Assert(s.machine0.Addresses(), tc.HasLen, 0)
	c.Assert(s.machine1.Addresses(), tc.HasLen, 0)

	addresses := []network.MachineAddress{
		network.NewMachineAddress("127.0.0.1"),
		network.NewMachineAddress("8.8.8.8"),
	}
	args := params.SetMachinesAddresses{MachineAddresses: []params.MachineAddresses{
		{Tag: "machine-1", Addresses: params.FromMachineAddresses(addresses...)},
		{Tag: "machine-0", Addresses: params.FromMachineAddresses(addresses...)},
		{Tag: "machine-42", Addresses: params.FromMachineAddresses(addresses...)},
	}}

	s.networkService.EXPECT().GetAllSpaces(gomock.Any())

	result, err := s.machiner.SetMachineAddresses(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	expectedAddresses := network.NewSpaceAddresses("8.8.8.8", "127.0.0.1")
	c.Assert(s.machine1.MachineAddresses(), tc.DeepEquals, expectedAddresses)
	err = s.machine0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine0.MachineAddresses(), tc.HasLen, 0)
}

func (s *machinerSuite) TestSetEmptyMachineAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	// Set some addresses so we can ensure they are removed.
	addresses := []network.MachineAddress{
		network.NewMachineAddress("127.0.0.1"),
		network.NewMachineAddress("8.8.8.8"),
	}
	args := params.SetMachinesAddresses{MachineAddresses: []params.MachineAddresses{
		{Tag: "machine-1", Addresses: params.FromMachineAddresses(addresses...)},
	}}
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Times(2)
	result, err := s.machiner.SetMachineAddresses(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})
	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.MachineAddresses(), tc.HasLen, 2)

	args.MachineAddresses[0].Addresses = nil
	result, err = s.machiner.SetMachineAddresses(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.MachineAddresses(), tc.HasLen, 0)
}

func (s *machinerSuite) TestWatch(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	loggo.GetLogger("juju.state.pool.txnwatcher").SetLogLevel(loggo.TRACE)
	loggo.GetLogger("juju.state.watcher").SetLogLevel(loggo.TRACE)

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

	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.Hostname(), tc.Equals, "thundering-herds", tc.Commentf("expected the machine hostname to be updated"))
}
