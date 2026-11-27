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
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID, true).
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
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID, true).
		Return(expectedAddresses, nil)

	// Call the function and check the result
	addresses, err := NewService(s.st, nil).GetMachineAddresses(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.DeepEquals, expectedAddresses)
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

	expectedError := errors.New("error while fetching public address")
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)
	s.st.EXPECT().GetNetNodePublicAddress(gomock.Any(), netNodeUUID).
		Return(network.SpaceAddress{}, expectedError)

	_, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *machineaddressSuite) TestGetMachinePublicAddressNoPublicAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)
	s.st.EXPECT().GetNetNodePublicAddress(gomock.Any(), netNodeUUID).
		Return(network.SpaceAddress{}, nil)

	_, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), machineUUID)
	c.Assert(err, tc.Satisfies, network.IsNoAddressError)
}

func (s *machineaddressSuite) TestGetMachinePublicAddressSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"
	publicAddress := network.NewSpaceAddress("8.8.8.8")
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)
	s.st.EXPECT().GetNetNodePublicAddress(gomock.Any(), netNodeUUID).
		Return(publicAddress, nil)

	address, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address, tc.DeepEquals, publicAddress)
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
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID, false).
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
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID, false).
		Return(addresses, nil)

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
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID, false).
		Return(addresses, nil)

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
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID, false).
		Return(addresses, nil)

	address, err := NewService(s.st, nil).GetMachinePrivateAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.DeepEquals, addresses[1])
}

// The service accepts the cloud-local fallback selected by state.
func (s *machineaddressSuite) TestGetMachinePublicAddressUsesStateFallback(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"
	expected := network.NewSpaceAddress("10.1.0.123")
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)
	s.st.EXPECT().GetNetNodePublicAddress(gomock.Any(), netNodeUUID).
		Return(expected, nil)

	address, err := NewService(s.st, nil).GetMachinePublicAddress(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address, tc.DeepEquals, expected)
}

// TestGetMachinePrivateAddressDoesNotPreferDefaultRouteSource checks that the
// private path keeps the original value ordering and does not use the
// default-route preference used for public and all-address selection.
func (s *machineaddressSuite) TestGetMachinePrivateAddressDoesNotPreferDefaultRouteSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID := "net-node-456"
	addresses := network.SpaceAddresses{
		network.NewSpaceAddress("10.1.0.123", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("10.0.0.123", network.WithScope(network.ScopeCloudLocal)),
	}

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).
		Return(netNodeUUID, nil)
	s.st.EXPECT().GetNetNodeAddresses(gomock.Any(), netNodeUUID, false).
		Return(addresses, nil)

	// Act
	address, err := NewService(s.st, nil).GetMachinePrivateAddress(c.Context(), machineUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address.Value, tc.Equals, "10.0.0.123")
}
