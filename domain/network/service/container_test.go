// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/network/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalnetwork "github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/testhelpers"
)

type containerSuite struct {
	testhelpers.IsolationSuite

	st *MockState

	hostUUID  machine.UUID
	guestUUID machine.UUID
	nodeUUID  string

	svc *Service
}

func TestContainerSuite(t *testing.T) {
	tc.Run(t, &containerSuite{})
}

func (s *containerSuite) TestDevicesToBridgeConflictingSpaceConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(
		[]internal.SpaceName{},
		[]internal.SpaceName{{
			UUID: "negative-space-uuid",
			Name: "negative-space",
		}},
		nil,
	)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(
		[]internal.SpaceName{{
			UUID: "negative-space-uuid",
			Name: "negative-space",
		}},
		nil,
	)

	_, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementConflict)
}

func (s *containerSuite) TestDevicesToBridgeSpaceReqsSatisfied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	// A bridge in the space means that connectivity is satisfied.
	exp.NICsInSpaces(c.Context(), s.nodeUUID, []string{spaceUUID}).Return(map[string][]network.NetInterface{
		spaceUUID: {{
			Name: "br-not-default-lxd",
			Type: corenetwork.BridgeDevice,
		}},
	}, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodProvider.String(), nil)

	nics, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(nics, tc.HasLen, 0)
}

func (s *containerSuite) TestDevicesToBridgeSpaceReqsUnsatisfiable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	// No devices in the space means the host can't
	// accommodate the guest space requirements.
	exp.NICsInSpaces(c.Context(), s.nodeUUID, []string{spaceUUID}).Return(nil, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodProvider.String(), nil)

	_, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementsUnsatisfiable)
}

func (s *containerSuite) TestDevicesToBridgeDeviceSatisfiesSpaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	// An ethernet device in the space can be bridged to satisfy the guest's
	// requirements. Loopback devices are not considered.
	exp.NICsInSpaces(c.Context(), s.nodeUUID, []string{spaceUUID}).Return(map[string][]network.NetInterface{
		spaceUUID: {
			{
				Name: "lo",
				Type: corenetwork.LoopbackDevice,
			},
			{
				Name:       "eth0",
				Type:       corenetwork.EthernetDevice,
				MACAddress: ptr("some-mac-address"),
			},
		},
	}, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodProvider.String(), nil)

	nics, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	c.Check(nics[0], tc.DeepEquals, network.DeviceToBridge{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
		MACAddress: "some-mac-address",
	})
}

func (s *containerSuite) TestDevicesToBridgeMultipleReqsMultipleDevsSatisfySpaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	spaceOne := "one-space-uuid"
	consSpaces := []internal.SpaceName{{
		UUID: spaceOne,
		Name: "one-space",
	}}

	spaceTwo := "two-space-uuid"
	boundSpaces := []internal.SpaceName{{
		UUID: spaceTwo,
		Name: "two-space",
	}}

	negativeSpaces := []internal.SpaceName{{
		UUID: "negative-space-uuid",
		Name: "negative-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(consSpaces, negativeSpaces, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(boundSpaces, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	// An ethernet device in the space can be bridged to satisfy one space
	// requirement and an existing bridge satisfies the other.
	exp.NICsInSpaces(c.Context(), s.nodeUUID, []string{spaceOne, spaceTwo}).Return(map[string][]network.NetInterface{
		spaceOne: {{
			Name:       "eth0",
			Type:       corenetwork.EthernetDevice,
			MACAddress: ptr("some-mac-address"),
		}},
		spaceTwo: {{
			Name: "br-not-default-lxd",
			Type: corenetwork.BridgeDevice,
		}},
	}, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodProvider.String(), nil)

	nics, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	c.Check(nics[0], tc.DeepEquals, network.DeviceToBridge{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
		MACAddress: "some-mac-address",
	})
}

func (s *containerSuite) TestDevicesToBridgeLocalBridgeReqsUnsatisfiable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	// The default LXD bridge cannot satisfy space requirements when the
	// container networking method is "provider".
	exp.NICsInSpaces(c.Context(), s.nodeUUID, []string{spaceUUID}).Return(map[string][]network.NetInterface{
		spaceUUID: {{
			Name: internalnetwork.DefaultLXDBridge,
			Type: corenetwork.BridgeDevice,
		}},
	}, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodProvider.String(), nil)

	_, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementsUnsatisfiable)
}

func (s *containerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = NewMockState(ctrl)
	c.Cleanup(func() { s.st = nil })
	return ctrl
}

func (s *containerSuite) setupServiceAndMachines(c *tc.C) {
	var err error

	s.hostUUID, err = machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.guestUUID, err = machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.nodeUUID = "net-node-uuid"

	s.svc = NewService(s.st, loggertesting.WrapCheckLog(c))
	c.Cleanup(func() { s.svc = nil })
}
