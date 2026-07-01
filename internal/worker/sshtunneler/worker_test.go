// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/sshtunneler"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	testhelpers.IsolationSuite

	sshModelService       *MockSSHModelService
	machineService        *MockMachineService
	controllerNodeService *MockControllerNodeService
}

func TestWorkerSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &workerSuite{})
	})
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.sshModelService = NewMockSSHModelService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)

	return ctrl
}

// stubServicesGetter is a minimal DomainServicesGetter stub for tests that
// exercise the worker construction path (not the model-scoped routing).
type stubServicesGetter struct{}

func (stubServicesGetter) ServicesForModel(_ context.Context, _ coremodel.UUID) (services.DomainServices, error) {
	return nil, errors.New("not implemented in stub")
}

func (s *workerSuite) newGetSSHService(svc SSHModelService) GetSSHServiceFunc {
	return func(_ context.Context, _ services.DomainServicesGetter, _ coremodel.UUID) (SSHModelService, error) {
		return svc, nil
	}
}

func (s *workerSuite) newGetMachineService(svc MachineService) GetMachineServiceFunc {
	return func(_ context.Context, _ services.DomainServicesGetter, _ coremodel.UUID) (MachineService, error) {
		return svc, nil
	}
}

func (s *workerSuite) TestWorkerStartStop(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewWorker(
		stubServicesGetter{},
		s.newGetSSHService(s.sshModelService),
		s.newGetMachineService(s.machineService),
		s.controllerNodeService,
		clock.WallClock,
	)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerExposesTracker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewWorker(
		stubServicesGetter{},
		s.newGetSSHService(s.sshModelService),
		s.newGetMachineService(s.machineService),
		s.controllerNodeService,
		clock.WallClock,
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	var tracker *sshtunneler.Tracker
	err = outputFunc(w, &tracker)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tracker, tc.NotNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestOutputFuncTypeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewWorker(
		stubServicesGetter{},
		s.newGetSSHService(s.sshModelService),
		s.newGetMachineService(s.machineService),
		s.controllerNodeService,
		clock.WallClock,
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	var wrongType string
	err = outputFunc(w, &wrongType)
	c.Assert(err, tc.ErrorMatches, `out should be \*sshtunneler\.Tracker; got \*string`)

	workertest.CleanKill(c, w)
}

// TestStateAdapterInsertSSHConnRequestUsesRequestModelUUID verifies that
// model-scoped routing uses the model UUID from the request, not a fixed UUID.
func (s *workerSuite) TestStateAdapterInsertSSHConnRequestUsesRequestModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelA := coremodel.UUID("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	modelB := coremodel.UUID("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	req := domainssh.SSHConnRequest{
		TunnelID:    "test-tunnel-id",
		MachineName: "0",
	}

	var calledWithUUID coremodel.UUID
	getter := func(_ context.Context, _ services.DomainServicesGetter, uuid coremodel.UUID) (SSHModelService, error) {
		calledWithUUID = uuid
		return s.sshModelService, nil
	}

	s.sshModelService.EXPECT().InsertSSHConnRequest(gomock.Any(), req).Return(nil)

	adapter := &connRequestStateAdapter{
		domainServicesGetter: stubServicesGetter{},
		getSSHService:        getter,
	}
	err := adapter.InsertSSHConnRequest(c.Context(), modelA, req)
	c.Assert(err, tc.ErrorIsNil)

	// Verify model UUID from the request was used for routing, not model B.
	c.Check(calledWithUUID, tc.Equals, modelA)
	c.Check(calledWithUUID, tc.Not(tc.Equals), modelB)
}

// TestStateAdapterMachineHostKeysRoutedByModelUUID verifies that machine host
// key lookup uses the model UUID argument for model-scoped routing and returns
// the SSH host keys from the machine service.
func (s *workerSuite) TestStateAdapterMachineHostKeysRoutedByModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := "8419cd78-4993-4c3a-928e-c646226beeee"
	machineUUID := coremachine.UUID("machine-uuid-0001")

	var calledWithUUID coremodel.UUID
	machineGetter := func(_ context.Context, _ services.DomainServicesGetter, uuid coremodel.UUID) (MachineService, error) {
		calledWithUUID = uuid
		return s.machineService, nil
	}
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(machineUUID, nil)
	s.machineService.EXPECT().GetSSHHostKeys(gomock.Any(), machineUUID).Return(
		[]string{"ssh-ed25519 AAAA... user@host"}, nil,
	)

	adapter := &machineStateAdapter{
		domainServicesGetter: stubServicesGetter{},
		getMachineService:    machineGetter,
	}

	keys, err := adapter.MachineHostKeys(c.Context(), modelUUID, "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keys, tc.DeepEquals, []string{"ssh-ed25519 AAAA... user@host"})
	c.Check(calledWithUUID, tc.Equals, coremodel.UUID(modelUUID))
}

// TestStateAdapterMachineHostKeysInvalidModelUUID verifies that an invalid
// model UUID causes an error during MachineHostKeys.
func (s *workerSuite) TestStateAdapterMachineHostKeysInvalidModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	adapter := &machineStateAdapter{
		domainServicesGetter: stubServicesGetter{},
		getMachineService:    s.newGetMachineService(s.machineService),
	}

	_, err := adapter.MachineHostKeys(c.Context(), "not-a-uuid", "0")
	c.Assert(err, tc.ErrorMatches, `invalid model UUID "not-a-uuid": .*`)
}

// TestControllerInfoAdapterAddresses verifies that controller addresses are
// retrieved from the controller-scoped service and returned as SpaceAddresses.
func (s *workerSuite) TestControllerInfoAdapterAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerNodeService.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(
		[]string{"10.0.0.1:17070", "10.0.0.2:17070"}, nil,
	)

	adapter := &controllerInfoAdapter{controllerNodeService: s.controllerNodeService}
	addrs, err := adapter.Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 2)
	c.Check(addrs[0].Value, tc.Equals, "10.0.0.1:17070")
	c.Check(addrs[1].Value, tc.Equals, "10.0.0.2:17070")
}

// TestControllerInfoAdapterAddressesError verifies that errors from the
// controller node service are wrapped and returned.
func (s *workerSuite) TestControllerInfoAdapterAddressesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerNodeService.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(
		nil, errors.New("db unavailable"),
	)

	adapter := &controllerInfoAdapter{controllerNodeService: s.controllerNodeService}
	_, err := adapter.Addresses(c.Context())
	c.Assert(err, tc.ErrorMatches, `failed to get controller addresses: db unavailable`)
}
