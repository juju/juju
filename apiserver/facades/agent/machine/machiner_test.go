// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/agent/machine"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

type machinerSuite struct {
	commonSuite

	clock clock.Clock

	machiner           *machine.MachinerAPI
	networkService     *MockNetworkService
	machineService     *MockMachineService
	applicationService *MockApplicationService
	statusService      *MockStatusService
	removalService     *MockRemovalService
	watcherRegistry    *MockWatcherRegistry
}

func TestMachinerSuite(t *testing.T) {
	tc.Run(t, &machinerSuite{})
}

func (s *machinerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = testclock.NewClock(time.Now())
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)

	c.Cleanup(func() {
		s.clock = nil
		s.watcherRegistry = nil
		s.networkService = nil
		s.machineService = nil
		s.applicationService = nil
		s.statusService = nil
		s.removalService = nil
	})

	return ctrl
}

func (s *machinerSuite) makeAPI(c *tc.C) {
	st := s.ControllerModel(c).State()
	// Create a machiner API for machine 1.
	machiner, err := machine.NewMachinerAPIForState(
		st,
		s.clock,
		s.ControllerDomainServices(c).ControllerConfig(),
		s.ControllerDomainServices(c).ControllerNode(),
		s.networkService,
		s.applicationService,
		s.machineService,
		s.statusService,
		s.removalService,
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
		st,
		clock.WallClock,
		s.ControllerDomainServices(c).ControllerConfig(),
		nil,
		s.networkService,
		s.applicationService,
		s.machineService,
		s.statusService,
		s.removalService,
		s.watcherRegistry,
		anAuthorizer,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.NotNil)
	c.Assert(aMachiner, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *machinerSuite) TestEnsureDead(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	machineName := coremachine.Name("1")
	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, nil)
	s.removalService.EXPECT().MarkMachineAsDead(gomock.Any(), machineUUID).Return(nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
	}}
	result, err := s.machiner.EnsureDead(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})
}

func (s *machinerSuite) TestEnsureDeadMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("1")).Return("", machineerrors.MachineNotFound)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
	}}
	result, err := s.machiner.EnsureDead(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *machinerSuite) TestSetStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	now := s.clock.Now()
	s.statusService.EXPECT().SetMachineStatus(gomock.Any(), coremachine.Name("1"), status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Since:   &now,
	})

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-1", Status: status.Error.String(), Info: "not really"},
		}}
	result, err := s.machiner.SetStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
}

func (s *machinerSuite) TestSetStatusMachineNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.makeAPI(c)

	now := s.clock.Now()
	s.statusService.EXPECT().SetMachineStatus(gomock.Any(), coremachine.Name("1"), status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Since:   &now,
	}).Return(machineerrors.MachineNotFound)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-1", Status: status.Error.String(), Info: "not really"},
		}}
	result, err := s.machiner.SetStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *machinerSuite) TestSetStatusInvalidTags(c *tc.C) {
	result, err := s.machiner.SetStatus(c.Context(), params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: "application-unknown"},
		{Tag: "invalid-tag"},
		{Tag: "unit-missing-1"},
		{Tag: ""},
		{Tag: "42"},
	}})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
	}})
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

func (s *machinerSuite) TestWatch(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.makeAPI(c)

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	watcher := watchertest.NewMockNotifyWatcher(ch)
	defer workertest.CleanKill(c, watcher)

	s.machineService.EXPECT().WatchMachineLife(gomock.Any(), coremachine.Name("1")).Return(watcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-42"},
		},
	}
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
