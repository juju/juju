// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/controllernode"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/errors"
)

type controllerSuite struct {
	baseSuite
}

func TestControllerSuite(t *testing.T) {
	tc.Run(t, &controllerSuite{})
}

func (s *controllerSuite) TestFinaliseIAASAgentUsesMachineIdentityAndNonce(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineName := machine.Name("42")
	machineUUID := machinetesting.GenUUID(c)
	params := instancecfg.StateInitializationParams{
		BootstrapMachineInstanceId:  instance.Id("replacement-instance"),
		BootstrapMachineDisplayName: "replacement-controller",
		BootstrapMachineHardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: new("amd64"), Mem: new(uint64(4096)),
		},
	}
	gomock.InOrder(
		s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, nil),
		s.agentPasswordService.EXPECT().SetMachinePassword(gomock.Any(), machineName, "agent-password").Return(nil),
		s.machineService.EXPECT().SetMachineCloudInstance(gomock.Any(), machineUUID,
			params.BootstrapMachineInstanceId, params.BootstrapMachineDisplayName,
			"selected-nonce", params.BootstrapMachineHardwareCharacteristics).Return(nil),
	)
	err := FinaliseIAASAgent(c.Context(), s.agentPasswordService, s.machineService,
		machineName, "selected-nonce", params, "agent-password")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *controllerSuite) TestFinaliseIAASAgentPasswordFailureDoesNotProvision(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineName := machine.Name("42")
	expected := errors.New("password failed")
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machinetesting.GenUUID(c), nil)
	s.agentPasswordService.EXPECT().SetMachinePassword(gomock.Any(), machineName, "agent-password").Return(expected)
	err := FinaliseIAASAgent(c.Context(), s.agentPasswordService, s.machineService,
		machineName, "selected-nonce", instancecfg.StateInitializationParams{}, "agent-password")
	c.Check(err, tc.ErrorIs, expected)
}

func (s *controllerSuite) TestFreshIAASIdentity(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("0")).Return(machineUUID, nil)
	s.agentPasswordService.EXPECT().SetMachinePassword(gomock.Any(), machine.Name("0"), "agent-password").Return(nil)
	s.machineService.EXPECT().SetMachineCloudInstance(gomock.Any(), machineUUID,
		instance.Id(""), "", agent.BootstrapNonce, (*instance.HardwareCharacteristics)(nil)).Return(nil)
	err := IAASAgentFinalizer(c.Context(), s.agentPasswordService, s.machineService,
		instancecfg.StateInitializationParams{}, "agent-password")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *controllerSuite) TestFinaliseK8sAgentUsesControllerIdentity(c *tc.C) {
	defer s.setupMocks(c).Finish()
	noncePath := filepath.Join(c.MkDir(), "nonce")
	c.Assert(os.WriteFile(noncePath, []byte("selected-nonce"), 0600), tc.ErrorIsNil)
	gomock.InOrder(
		s.agentPasswordService.EXPECT().SetControllerNodePassword(gomock.Any(), "7", "agent-password").Return(nil),
		s.agentPasswordService.EXPECT().EnsureControllerNodeNonce(gomock.Any(), "7", "selected-nonce").Return("persisted-nonce", nil),
	)
	err := FinaliseK8sAgent(c.Context(), s.agentPasswordService, "7", "agent-password", noncePath)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *controllerSuite) TestFinaliseK8sAgentWithoutNonceFile(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.agentPasswordService.EXPECT().SetControllerNodePassword(gomock.Any(), "7", "agent-password").Return(nil)
	err := FinaliseK8sAgent(c.Context(), s.agentPasswordService, "7", "agent-password", filepath.Join(c.MkDir(), "absent"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *controllerSuite) TestFinaliseK8sAgentNonceFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()
	expected := errors.New("nonce failed")
	noncePath := filepath.Join(c.MkDir(), "nonce")
	c.Assert(os.WriteFile(noncePath, []byte("selected-nonce"), 0600), tc.ErrorIsNil)
	s.agentPasswordService.EXPECT().SetControllerNodePassword(gomock.Any(), "7", "agent-password").Return(nil)
	s.agentPasswordService.EXPECT().EnsureControllerNodeNonce(gomock.Any(), "7", "selected-nonce").Return("", expected)
	err := FinaliseK8sAgent(c.Context(), s.agentPasswordService, "7", "agent-password", noncePath)
	c.Check(err, tc.ErrorIs, expected)
}

func (s *controllerSuite) TestFreshK8sIdentity(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Fail before reading the real nonce file; its path belongs to the pod.
	expected := errors.New("password failed")
	s.agentPasswordService.EXPECT().SetControllerNodePassword(gomock.Any(), "0", "agent-password").Return(expected)
	err := K8sAgentFinalizer(c.Context(), s.agentPasswordService, nil,
		instancecfg.StateInitializationParams{}, "agent-password")
	c.Check(err, tc.ErrorIs, expected)
}

func (s *controllerSuite) TestInitialiseAPIHostPortsUsesControllerIdentity(c *tc.C) {
	s.assertAPIHostPorts(c, nil, nil)
}

func (s *controllerSuite) TestInitialiseAPIHostPortsWithoutManagementSpace(c *tc.C) {
	s.assertAPIHostPorts(c, networkerrors.SpaceNotFound, nil)
}

func (s *controllerSuite) TestInitialiseAPIHostPortsPublishFailure(c *tc.C) {
	s.assertAPIHostPorts(c, nil, errors.New("publishing failed"))
}

func (s *controllerSuite) assertAPIHostPorts(c *tc.C, managementSpaceErr, publishErr error) {
	defer s.setupMocks(c).Finish()
	space := network.SpaceInfo{
		ID: "17f770f6-ff91-4c3c-bce7-253bc1e4058b", Name: "management",
		Subnets: network.SubnetInfos{{CIDR: "10.0.0.0/24"}, {CIDR: "2001:db8::/64"}},
	}
	addresses := network.NewMachineAddresses([]string{"10.0.0.42", "2001:db8::42"}, network.WithScope(network.ScopeCloudLocal))
	addresses[0].CIDR = space.Subnets[0].CIDR
	addresses[1].CIDR = space.Subnets[1].CIDR
	providerAddresses := addresses.AsProviderAddresses()
	expected := network.SpaceHostPorts{
		{SpaceAddress: network.SpaceAddress{MachineAddress: addresses[0], SpaceID: space.ID}, NetPort: 17070},
		{SpaceAddress: network.SpaceAddress{MachineAddress: addresses[1], SpaceID: space.ID}, NetPort: 17070},
	}
	managementSpace := &space
	if managementSpaceErr != nil {
		managementSpace = nil
	}
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(network.SpaceInfos{space}, nil)
	s.networkService.EXPECT().SpaceByName(gomock.Any(), space.Name).Return(managementSpace, managementSpaceErr)
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), controllernode.SetAPIAddressArgs{
		MgmtSpace:    managementSpace,
		APIAddresses: map[string]network.SpaceHostPorts{"7": expected},
	}).Return(publishErr)
	err := InitialiseAPIHostPorts(c.Context(), s.controllerNodeService, s.networkService, "7",
		controller.Config{controller.JujuManagementSpace: "management"}, providerAddresses, 17070)
	if publishErr == nil {
		c.Assert(err, tc.ErrorIsNil)
	} else {
		c.Check(err, tc.ErrorIs, publishErr)
	}
}

func (s *controllerSuite) TestInitialiseAPIHostPortsSpaceLookupFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()
	expected := errors.New("space lookup failed")
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, expected)
	err := InitialiseAPIHostPorts(c.Context(), s.controllerNodeService, s.networkService, "7",
		controller.Config{}, nil, 17070)
	c.Check(err, tc.ErrorIs, expected)
}

func (s *controllerSuite) TestFinaliseK8sAgentPasswordBeforeNonceRead(c *tc.C) {
	defer s.setupMocks(c).Finish()
	noncePath := filepath.Join(c.MkDir(), "nonce")
	s.agentPasswordService.EXPECT().SetControllerNodePassword(gomock.Any(), "7", "agent-password").
		DoAndReturn(func(context.Context, string, string) error {
			return os.WriteFile(noncePath, []byte("selected-nonce"), 0600)
		})
	s.agentPasswordService.EXPECT().EnsureControllerNodeNonce(gomock.Any(), "7", "selected-nonce").Return("selected-nonce", nil)
	err := FinaliseK8sAgent(c.Context(), s.agentPasswordService, "7", "agent-password", noncePath)
	c.Assert(err, tc.ErrorIsNil)
}
