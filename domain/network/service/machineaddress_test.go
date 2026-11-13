// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type machineaddressSuite struct {
	testhelpers.IsolationSuite

	st *MockState
}

func TestMachineAddressSuite(t *testing.T) {
	tc.Run(t, &machineaddressSuite{})
}

func (s *machineaddressSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	return ctrl
}

func (s *machineaddressSuite) TestGetMachineAddressesInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Create an invalid UUID
	invalidUUID := machine.UUID("not-a-uuid")

	// Call the function with the invalid UUID
	_, err := NewService(s.st, nil).GetMachineAddresses(c.Context(), invalidUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *machineaddressSuite) TestGetMachineAddressesErrorGettingNetNodeUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedError := errors.New("net node not found")
	// Create a valid UUID
	machineUUID := machinetesting.GenUUID(c)

	// Expect a call to GetMachineNetNodeUUID and return an error
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return("", expectedError)

	// Call the function and check the error
	_, err := NewService(s.st, nil).GetMachineAddresses(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *machineaddressSuite) TestGetMachineAddressesErrorGettingAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedError := errors.New("error while fetching addresses")
	// Create a valid UUID
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	// Expect a call to GetMachineNetNodeUUID and return a net node UUID
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)

	// Expect a call to GetNetNodeAddresses and return an error
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return(nil, expectedError)

	// Call the function and check the error
	_, err := NewService(s.st, nil).GetMachineAddresses(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *machineaddressSuite) TestGetMachineAddressesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Create a valid UUID
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	// Create some addresses to return
	expectedAddresses := network.NewSpaceAddresses(
		"192.168.1.1",
		"10.0.0.1",
	)

	// Expect a call to GetMachineNetNodeUUID and return a net node UUID
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)

	// Expect a call to GetNetNodeAddresses and return the addresses
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return(toDeviceAddresses(expectedAddresses), nil)

	// Call the function and check the result
	addresses, err := NewService(s.st, nil).GetMachineAddresses(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.DeepEquals, expectedAddresses)
}

// TestGetMachineAddressesMarksDefaultRouteSource checks that only the primary
// address on the default route device is marked as the default route source,
// and that the addresses are returned in the order they were retrieved.
func (s *machineaddressSuite) TestGetMachineAddressesMarksDefaultRouteSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	eth1Addr := defaultRouteDeviceAddress("10.0.0.123", "10.0.0.0/24", "eth1-uuid")
	eth1Addr.IsDefaultGatewayDevice = false
	eth1Addr.GatewayAddress = ""
	eth0Addr := defaultRouteDeviceAddress("10.1.0.123", "10.1.0.0/24", "eth0-uuid")
	eth0Secondary := defaultRouteDeviceAddress("10.1.0.124", "10.1.0.0/24", "eth0-uuid")
	eth0Secondary.IsSecondary = true

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return([]network.DeviceAddress{
			eth1Addr, eth0Addr, eth0Secondary,
		}, nil)

	// Act
	addresses, err := NewService(s.st, nil).GetMachineAddresses(c.Context(), machineUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	expectedSource := eth0Addr.SpaceAddress
	expectedSource.IsDefaultRouteSource = true
	c.Check(addresses, tc.DeepEquals, network.SpaceAddresses{
		eth1Addr.SpaceAddress,
		expectedSource,
		eth0Secondary.SpaceAddress,
	})
}

func (s *machineaddressSuite) TestGetMachinePublicAddressInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Create an invalid UUID
	invalidUUID := machine.UUID("not-a-uuid")

	// Call the function with the invalid UUID
	_, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), invalidUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *machineaddressSuite) TestGetMachinePublicAddressErrorGettingNetNodeUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedError := errors.New("net node not found")
	// Create a valid UUID
	machineUUID := machinetesting.GenUUID(c)

	// Expect a call to GetMachineNetNodeUUID and return an error
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return("", expectedError)

	// Call the function and check the error
	_, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *machineaddressSuite) TestGetMachinePublicAddressErrorGettingAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedError := errors.New("error while fetching addresses")
	// Create a valid UUID
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	// Expect a call to GetMachineNetNodeUUID and return a net node UUID
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)

	// Expect a call to GetNetNodeAddresses and return an error
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return(nil, expectedError)

	// Call the function and check the error
	_, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *machineaddressSuite) TestGetMachinePublicAddressNoPublicAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Create a valid UUID
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	// Create addresses with a scope that won't match public
	addresses := network.SpaceAddresses{
		network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeLinkLocal)),
	}

	// Expect a call to GetMachineNetNodeUUID and return a net node UUID
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)

	// Expect a call to GetNetNodeAddresses and return the addresses
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return(toDeviceAddresses(addresses), nil)

	// Call the function and check the error
	_, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorMatches, "no public address\\(es\\)")
}

func (s *machineaddressSuite) TestGetMachinePublicAddressSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Create a valid UUID
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	// Create addresses with a public address
	publicAddress := network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic))
	addresses := network.SpaceAddresses{
		network.NewSpaceAddress("10.0.0.1"), // Private address
		publicAddress,                       // Public address
	}

	// Expect a call to GetMachineNetNodeUUID and return a net node UUID
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)

	// Expect a call to GetNetNodeAddresses and return the addresses
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return(toDeviceAddresses(addresses), nil)

	// Call the function and check the result
	address, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.DeepEquals, publicAddress)
}

func (s *machineaddressSuite) TestGetMachinePrivateAddressInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Create an invalid UUID
	invalidUUID := machine.UUID("not-a-uuid")

	// Call the function with the invalid UUID
	_, err := NewService(s.st, nil).GetMachinePrivateAddress(c.Context(), invalidUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *machineaddressSuite) TestGetMachinePrivateAddressErrorGettingNetNodeUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedError := errors.New("net node not found")
	// Create a valid UUID
	machineUUID := machinetesting.GenUUID(c)

	// Expect a call to GetMachineNetNodeUUID and return an error
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return("", expectedError)

	// Call the function and check the error
	_, err := NewService(s.st, nil).GetMachinePrivateAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *machineaddressSuite) TestGetMachinePrivateAddressErrorGettingAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedError := errors.New("error while fetching addresses")
	// Create a valid UUID
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	// Expect a call to GetMachineNetNodeUUID and return a net node UUID
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)

	// Expect a call to GetNetNodeAddresses and return an error
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return(nil, expectedError)

	// Call the function and check the error
	_, err := NewService(s.st, nil).GetMachinePrivateAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *machineaddressSuite) TestGetMachinePrivateAddressNoPrivateAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Create a valid UUID
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	// Create addresses with a scope that won't match private
	addresses := network.SpaceAddresses{
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopeLinkLocal)),
	}

	// Expect a call to GetMachineNetNodeUUID and return a net node UUID
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)

	// Expect a call to GetNetNodeAddresses and return the addresses
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return(toDeviceAddresses(addresses), nil)

	// Call the function and check the error
	_, err := NewService(s.st, nil).GetMachinePrivateAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorMatches, "no private address\\(es\\)")
}

func (s *machineaddressSuite) TestGetMachinePrivateAddressSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Create a valid UUID
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	// Create addresses with a private address
	privateAddress := network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal))
	addresses := network.SpaceAddresses{
		network.NewSpaceAddress("8.8.8.8"), // Public address
		privateAddress,                     // Private address
	}

	// Expect a call to GetMachineNetNodeUUID and return a net node UUID
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)

	// Expect a call to GetNetNodeAddresses and return the addresses
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return(toDeviceAddresses(addresses), nil)

	// Call the function and check the result
	address, err := NewService(s.st, nil).GetMachinePrivateAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.DeepEquals, privateAddress)
}

func (s *machineaddressSuite) TestGetMachinePrivateAddressSortsMatches(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	addresses := network.SpaceAddresses{
		network.NewSpaceAddress("10.0.0.9", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("10.0.0.2", network.WithScope(network.ScopeCloudLocal)),
	}

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return(toDeviceAddresses(addresses), nil)

	address, err := NewService(s.st, nil).GetMachinePrivateAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.DeepEquals, addresses[1])
}

// TestGetMachinePublicAddressSortsMatches verifies that when multiple public
// addresses are available, GetMachinePublicAddress always returns the
// lexicographically smallest value, ensuring consistent results regardless
// of the order in which addresses are stored.
func (s *machineaddressSuite) TestGetMachinePublicAddressSortsMatches(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	// Place the lexicographically larger address first to confirm sorting.
	addresses := network.SpaceAddresses{
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("1.1.1.1", network.WithScope(network.ScopePublic)),
	}

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return(toDeviceAddresses(addresses), nil)

	address, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	// "1.1.1.1" sorts before "8.8.8.8", so it must always be returned first.
	c.Check(address, tc.DeepEquals, addresses[1])
}

func (s *machineaddressSuite) TestGetMachinePublicAddressPrioritizeScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	// Place the lexicographically larger and internal addresses first to confirm sorting.
	addresses := network.SpaceAddresses{
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("1.1.1.1", network.WithScope(network.ScopeMachineLocal)),
		network.NewSpaceAddress("2.2.2.2", network.WithScope(network.ScopePublic)),
	}

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return(toDeviceAddresses(addresses), nil)

	address, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	// "2.2.2.2" sorts before "8.8.8.8", and "1.1.1.1" is not public.
	c.Check(address, tc.DeepEquals, addresses[2])
}

// TestGetMachinePublicAddressPrefersDefaultRouteSource checks that, with no
// public addresses, the cloud-local address on the default route device is
// preferred over a lower-valued cloud-local address on another device.
func (s *machineaddressSuite) TestGetMachinePublicAddressPrefersDefaultRouteSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	eth1Addr := defaultRouteDeviceAddress("10.0.0.123", "10.0.0.0/24", "eth1-uuid")
	eth1Addr.IsDefaultGatewayDevice = false
	eth1Addr.GatewayAddress = ""
	eth0Addr := defaultRouteDeviceAddress("10.1.0.123", "10.1.0.0/24", "eth0-uuid")

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return([]network.DeviceAddress{eth1Addr, eth0Addr}, nil)

	// Act
	address, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), machineUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	expected := eth0Addr.SpaceAddress
	expected.IsDefaultRouteSource = true
	c.Check(address, tc.DeepEquals, expected)
}

// TestGetMachinePrivateAddressPrefersDefaultRouteSource checks that the
// cloud-local address on the default route device is preferred over a
// lower-valued cloud-local address on another device.
func (s *machineaddressSuite) TestGetMachinePrivateAddressPrefersDefaultRouteSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"

	eth1Addr := defaultRouteDeviceAddress("10.0.0.123", "10.0.0.0/24", "eth1-uuid")
	eth1Addr.IsDefaultGatewayDevice = false
	eth1Addr.GatewayAddress = ""
	eth0Addr := defaultRouteDeviceAddress("10.1.0.123", "10.1.0.0/24", "eth0-uuid")

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID).
		Return([]network.DeviceAddress{eth1Addr, eth0Addr}, nil)

	// Act
	address, err := NewService(s.st, nil).GetMachinePrivateAddress(c.Context(), machineUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	expected := eth0Addr.SpaceAddress
	expected.IsDefaultRouteSource = true
	c.Check(address, tc.DeepEquals, expected)
}
