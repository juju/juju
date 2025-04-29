// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&machinerSuite{})

func (s *machinerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = NewMockWatcherRegistry(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	return ctrl
}

func (s *machinerSuite) makeAPI(c *gc.C) {
	st := s.ControllerModel(c).State()
	// Create a machiner API for machine 1.
	machiner, err := machine.NewMachinerAPIForState(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)
	s.machiner = machiner
}

func (s *machinerSuite) TestMachinerFailsWithNonMachineAgentUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("ubuntu/1")
	st := s.ControllerModel(c).State()
	aMachiner, err := machine.NewMachinerAPIForState(
		context.Background(),
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
	c.Assert(err, gc.NotNil)
	c.Assert(aMachiner, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *machinerSuite) TestSetStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	now := time.Now()

	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Since:   &now,
	}
	err := s.machine0.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Stopped,
		Message: "foo",
		Since:   &now,
	}
	err = s.machine1.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-1", Status: status.Error.String(), Info: "not really"},
			{Tag: "machine-0", Status: status.Stopped.String(), Info: "foobar"},
			{Tag: "machine-42", Status: status.Started.String(), Info: "blah"},
		}}
	result, err := s.machiner.SetStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify machine 0 - no change.
	statusInfo, err := s.machine0.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Started)
	c.Assert(statusInfo.Message, gc.Equals, "blah")
	// ...machine 1 is fine though.
	statusInfo, err = s.machine1.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Error)
	c.Assert(statusInfo.Message, gc.Equals, "not really")
}

func (s *machinerSuite) TestLife(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	err := s.machine1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine1.Life(), gc.Equals, state.Dead)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.machiner.Life(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *machinerSuite) TestEnsureDead(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	c.Assert(s.machine0.Life(), gc.Equals, state.Alive)
	c.Assert(s.machine1.Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	s.machineService.EXPECT().EnsureDeadMachine(gomock.Any(), coremachine.Name("1")).Return(nil).Times(2)
	result, err := s.machiner.EnsureDead(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine0.Life(), gc.Equals, state.Alive)
	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine1.Life(), gc.Equals, state.Dead)

	// Try it again on a Dead machine; should work.
	args = params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	}
	result, err = s.machiner.EnsureDead(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	// Verify Life is unchanged.
	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine1.Life(), gc.Equals, state.Dead)
}

func (s *machinerSuite) TestSetMachineAddresses(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	c.Assert(s.machine0.Addresses(), gc.HasLen, 0)
	c.Assert(s.machine1.Addresses(), gc.HasLen, 0)

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

	result, err := s.machiner.SetMachineAddresses(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	expectedAddresses := network.NewSpaceAddresses("8.8.8.8", "127.0.0.1")
	c.Assert(s.machine1.MachineAddresses(), gc.DeepEquals, expectedAddresses)
	err = s.machine0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine0.MachineAddresses(), gc.HasLen, 0)
}

func (s *machinerSuite) TestSetEmptyMachineAddresses(c *gc.C) {
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
	result, err := s.machiner.SetMachineAddresses(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})
	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine1.MachineAddresses(), gc.HasLen, 2)

	args.MachineAddresses[0].Addresses = nil
	result, err = s.machiner.SetMachineAddresses(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine1.MachineAddresses(), gc.HasLen, 0)
}

func (s *machinerSuite) TestWatch(c *gc.C) {
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
	result, err := s.machiner.Watch(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *machinerSuite) TestRecordAgentStartInformation(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	args := params.RecordAgentStartInformationArgs{Args: []params.RecordAgentStartInformationArg{
		{Tag: "machine-1", Hostname: "thundering-herds"},
		{Tag: "machine-0", Hostname: "eldritch-octopii"},
		{Tag: "machine-42", Hostname: "missing-gem"},
	}}

	result, err := s.machiner.RecordAgentStartInformation(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine1.Hostname(), gc.Equals, "thundering-herds", gc.Commentf("expected the machine hostname to be updated"))
}
