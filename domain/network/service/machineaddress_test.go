// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

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
		Return(expectedAddresses, nil)

	// Call the function and check the result
	addresses, err := NewService(s.st, nil).GetMachineAddresses(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.DeepEquals, expectedAddresses)
}
