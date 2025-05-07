// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerprovisioner

import (
	"context"
	"errors"
	"sync"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	provisionermocks "github.com/juju/juju/api/agent/provisioner/mocks"
	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/network"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/container/factory"
	"github.com/juju/juju/internal/container/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type containerSetupSuite struct {
	coretesting.BaseSuite

	modelUUID      uuid.UUID
	controllerUUID uuid.UUID

	initialiser *testing.MockInitialiser
	caller      *provisionermocks.MockAPICaller
	machine     *provisionermocks.MockMachineProvisioner
	manager     *testing.MockManager

	machineLock *fakeMachineLock
}

func (s *containerSetupSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.modelUUID = uuid.MustNewUUID()
	s.controllerUUID = uuid.MustNewUUID()

	s.machineLock = &fakeMachineLock{}
}

var _ = tc.Suite(&containerSetupSuite{})

func (s *containerSetupSuite) TestInitialiseContainersLXD(c *tc.C) {
	s.testInitialiseContainers(c, instance.LXD)
}

func (s *containerSetupSuite) testInitialiseContainers(c *tc.C, containerType instance.ContainerType) {
	defer s.patch(c).Finish()

	s.expectContainerManagerConfig(containerType)
	s.initialiser.EXPECT().Initialise().Return(nil)

	s.PatchValue(
		&factory.NewContainerManager,
		func(forType instance.ContainerType, conf container.ManagerConfig) (container.Manager, error) {
			return s.manager, nil
		})

	cs := s.setUpContainerSetup(c, containerType)
	abort := make(chan struct{})
	close(abort)
	err := cs.initialiseContainers(context.Background(), abort)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *containerSetupSuite) TestInitialiseContainerProvisonerLXD(c *tc.C) {
	s.testInitialiseContainers(c, instance.LXD)
}

func (s *containerSetupSuite) TestContainerManagerConfigError(c *tc.C) {
	defer s.patch(c).Finish()

	s.caller.EXPECT().APICall(
		gomock.Any(),
		"Provisioner", 666, "", "ContainerManagerConfig", params.ContainerManagerConfigParams{Type: "lxd"}, gomock.Any()).Return(
		errors.New("boom"))

	cs := s.setUpContainerSetup(c, instance.LXD)
	abort := make(chan struct{})
	close(abort)
	err := cs.initialiseContainers(context.Background(), abort)
	c.Assert(err, tc.ErrorMatches, ".*generating container manager config: boom")
}

func (s *containerSetupSuite) setUpContainerSetup(c *tc.C, containerType instance.ContainerType) *ContainerSetup {
	pState := apiprovisioner.NewClient(s.caller)

	cfg, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             agent.DefaultPaths,
			Tag:               s.machine.MachineTag(),
			UpgradedToVersion: jujuversion.Current,
			Password:          "password",
			Nonce:             "nonce",
			APIAddresses:      []string{"0.0.0.0:12345"},
			CACert:            coretesting.CACert,
			Controller:        names.NewControllerTag(s.controllerUUID.String()),
			Model:             names.NewModelTag(s.modelUUID.String()),
		})
	c.Assert(err, tc.ErrorIsNil)

	args := ContainerSetupParams{
		Logger:        loggertesting.WrapCheckLog(c),
		ContainerType: containerType,
		MachineZone:   s.machine,
		MTag:          s.machine.MachineTag(),
		Provisioner:   pState,
		Config:        cfg,
		MachineLock:   s.machineLock,
		GetNetConfig: func(_ network.ConfigSource) (network.InterfaceInfos, error) {
			return nil, nil
		},
	}

	return NewContainerSetup(args)
}

func (s *containerSetupSuite) patch(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.initialiser = testing.NewMockInitialiser(ctrl)
	s.caller = provisionermocks.NewMockAPICaller(ctrl)
	s.caller.EXPECT().BestFacadeVersion("Provisioner").Return(666)
	s.machine = provisionermocks.NewMockMachineProvisioner(ctrl)
	s.manager = testing.NewMockManager(ctrl)

	s.machine.EXPECT().MachineTag().Return(names.NewMachineTag("0")).AnyTimes()

	s.PatchValue(GetContainerInitialiser, func(instance.ContainerType, map[string]string, containermanager.NetworkingMethod) (container.Initialiser, error) {
		return s.initialiser, nil
	})

	return ctrl
}

// expectContainerManagerConfig sets up expectations associated with
// acquisition and decoration of container manager configuration.
func (s *containerSetupSuite) expectContainerManagerConfig(cType instance.ContainerType) {
	resultSource := params.ContainerManagerConfig{
		ManagerConfig: map[string]string{"model-uuid": s.modelUUID.String()},
	}
	s.caller.EXPECT().APICall(
		gomock.Any(),
		"Provisioner", 666, "", "ContainerManagerConfig", params.ContainerManagerConfigParams{Type: cType}, gomock.Any(),
	).SetArg(6, resultSource).MinTimes(1)
}

type fakeMachineLock struct {
	mu sync.Mutex
}

func (f *fakeMachineLock) Acquire(spec machinelock.Spec) (func(), error) {
	f.mu.Lock()
	return func() {
		f.mu.Unlock()
	}, nil
}

func (f *fakeMachineLock) Report(opts ...machinelock.ReportOption) (string, error) {
	return "", nil
}
