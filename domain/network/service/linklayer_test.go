// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremachine "github.com/juju/juju/core/machine"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type linkLayerBaseSuite struct {
	st *MockState
}

func (s *linkLayerBaseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = NewMockState(ctrl)
	c.Cleanup(func() { s.st = nil })
	return ctrl
}

type linkLayerMigrationSuite struct {
	linkLayerBaseSuite
}

func TestLinkLayerMigrationSuite(t *testing.T) {
	tc.Run(t, &linkLayerMigrationSuite{})
}

func (s *linkLayerMigrationSuite) TestImportLinkLayerDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	netNodeUUID := uuid.MustNewUUID().String()
	nameMap := map[string]string{
		"88": netNodeUUID,
	}
	args := []internal.ImportLinkLayerDevice{
		{MachineID: "88"},
	}
	expectedArgs := args
	expectedArgs[0].NetNodeUUID = netNodeUUID
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().ImportLinkLayerDevices(gomock.Any(), args).Return(nil)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *linkLayerMigrationSuite) TestImportLinkLayerDevicesMachines(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nil,
		errors.New("boom"))
	args := []internal.ImportLinkLayerDevice{
		{
			MachineID: "88",
		},
	}

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert: error from AllMachinesAndNetNodes returned.
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *linkLayerMigrationSuite) TestImportLinkLayerDevicesNoContent(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(),
		[]internal.ImportLinkLayerDevice{})

	// Assert: no failure if no data provided.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *linkLayerMigrationSuite) TestDeleteImportedLinkLayerDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.st.EXPECT().DeleteImportedLinkLayerDevices(gomock.Any()).Return(errors.New("boom"))

	// Act
	err := s.migrationService(c).DeleteImportedLinkLayerDevices(c.Context())

	// Assert: the error from DeleteImportedLinkLayerDevices is passed
	// through to the caller.
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *linkLayerMigrationSuite) migrationService(c *tc.C) *MigrationService {
	return NewMigrationService(s.st, loggertesting.WrapCheckLog(c))
}

func stringPtr(s string) *string {
	return &s
}

type linkLayerMergeSuite struct {
	linkLayerBaseSuite
}

func (s *linkLayerMergeSuite) service(c *tc.C) *Service {
	return NewService(s.st, loggertesting.WrapCheckLog(c))
}

func TestLinkLayerMergeSuite(t *testing.T) {
	tc.Run(t, &linkLayerMergeSuite{})
}
func (s *linkLayerMergeSuite) TestMergeLinkLayerDevicesInvalidMachineUUID(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	invalidUUID := coremachine.UUID("invalid-uuid")

	// Act
	err := s.service(c).MergeLinkLayerDevice(c.Context(), invalidUUID, nil)

	// Assert
	c.Assert(err, tc.ErrorMatches,
		`invalid machine UUID: id "invalid-uuid" not valid`)
}

func (s *linkLayerMergeSuite) TestMergeLinkLayerDevicesError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	machineUUID := coremachine.UUID(uuid.MustNewUUID().String())
	incoming := []domainnetwork.NetInterface{{}, {}}
	stateErr := errors.New("boom")

	s.st.EXPECT().MergeLinkLayerDevice(gomock.Any(), machineUUID.String(),
		incoming).Return(stateErr)

	// Act
	err := s.service(c).MergeLinkLayerDevice(c.Context(), machineUUID, incoming)

	// Assert
	c.Assert(err, tc.ErrorIs, stateErr)
}

func (s *linkLayerMergeSuite) TestMergeLinkLayerDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	machineUUID := coremachine.UUID(uuid.MustNewUUID().String())
	incoming := []domainnetwork.NetInterface{
		{},
		{},
	}
	s.st.EXPECT().MergeLinkLayerDevice(gomock.Any(), machineUUID.String(),
		incoming).Return(nil)

	// Act
	err := s.service(c).MergeLinkLayerDevice(c.Context(), machineUUID, incoming)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}
