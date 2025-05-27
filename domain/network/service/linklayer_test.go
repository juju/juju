// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type linkLayerSuite struct {
	testhelpers.IsolationSuite

	st *MockState
}

func (s *linkLayerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = NewMockState(ctrl)
	c.Cleanup(func() { s.st = nil })
	return ctrl
}

func (s *linkLayerSuite) service(c *tc.C) *Service {
	return NewService(s.st, loggertesting.WrapCheckLog(c))
}

func TestLinkLayerSuite(t *testing.T) {
	tc.Run(t, &linkLayerSuite{})
}

func (s *linkLayerSuite) TestImportLinkLayerDevices(c *tc.C) {
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

func (s *linkLayerSuite) TestImportLinkLayerDevicesMachines(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nil, errors.New("boom"))
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

func (s *linkLayerSuite) TestImportLinkLayerDevicesNoContent(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), []internal.ImportLinkLayerDevice{})

	// Assert: no failure if no data provided.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *linkLayerSuite) TestDeleteImportedLinkLayerDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.st.EXPECT().DeleteImportedLinkLayerDevices(gomock.Any()).Return(errors.New("boom"))

	// Act
	err := s.migrationService(c).DeleteImportedLinkLayerDevices(c.Context())

	// Assert: the error from DeleteImportedLinkLayerDevices is passed
	// through to the caller.
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *linkLayerSuite) migrationService(c *tc.C) *MigrationService {
	return NewMigrationService(s.st, loggertesting.WrapCheckLog(c))
}

func (s *linkLayerSuite) TestSetMachineNetConfigBadUUIDError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machine.UUID("bad-machine-uuid")

	err := s.service(c).SetMachineNetConfig(c.Context(), mUUID, nil)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *linkLayerSuite) TestSetMachineNetConfigNodeNotFoundError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID, err := machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), mUUID.String()).Return("", machineerrors.MachineNotFound)

	nics := []network.NetInterface{{Name: "eth0"}}

	err = s.service(c).SetMachineNetConfig(c.Context(), mUUID, nics)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *linkLayerSuite) TestSetMachineNetConfigSetCallError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	nUUID := "set-node-uuid"
	mUUID, err := machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	nics := []network.NetInterface{{Name: "eth0"}}

	exp := s.st.EXPECT()
	exp.GetMachineNetNodeUUID(gomock.Any(), mUUID.String()).Return(nUUID, nil)
	exp.SetMachineNetConfig(gomock.Any(), nUUID, nics).Return(errors.New("boom"))

	err = s.service(c).SetMachineNetConfig(c.Context(), mUUID, nics)
	c.Assert(err, tc.ErrorMatches, "setting net config for machine .* boom")
}

func (s *linkLayerSuite) TestSetMachineNetConfigEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID, err := machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.service(c).SetMachineNetConfig(c.Context(), mUUID, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *linkLayerSuite) TestSetMachineNetConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ctx := c.Context()

	nUUID := "set-node-uuid"
	mUUID, err := machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	nics := []network.NetInterface{
		{
			Name: "eth0",
			Addrs: []network.NetAddr{
				{
					InterfaceName: "eth0",
					AddressValue:  "10.0.0.5/16",
					AddressType:   corenetwork.IPv4Address,
					ConfigType:    corenetwork.ConfigDHCP,
					Origin:        corenetwork.OriginMachine,
					Scope:         corenetwork.ScopeCloudLocal,
				},
			},
		},
	}

	exp := s.st.EXPECT()
	exp.GetMachineNetNodeUUID(gomock.Any(), mUUID.String()).Return(nUUID, nil)
	exp.SetMachineNetConfig(gomock.Any(), nUUID, nics).Return(nil)

	err = s.service(c).SetMachineNetConfig(ctx, mUUID, nics)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *linkLayerSuite) TestMergeLinkLayerDevicesInvalidMachineUUID(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	invalidUUID := machine.UUID("invalid-uuid")

	// Act
	err := s.service(c).MergeLinkLayerDevice(c.Context(), invalidUUID, nil)

	// Assert
	c.Assert(err, tc.ErrorMatches, `invalid machine UUID: id "invalid-uuid" not valid`)
}

func (s *linkLayerSuite) TestMergeLinkLayerDevicesError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	machineUUID := machine.UUID(uuid.MustNewUUID().String())
	incoming := []network.NetInterface{{}, {}}
	stateErr := errors.New("boom")

	s.st.EXPECT().MergeLinkLayerDevice(gomock.Any(), machineUUID.String(),
		incoming).Return(stateErr)

	// Act
	err := s.service(c).MergeLinkLayerDevice(c.Context(), machineUUID, incoming)

	// Assert
	c.Assert(err, tc.ErrorIs, stateErr)
}

func (s *linkLayerSuite) TestMergeLinkLayerDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	machineUUID := machine.UUID(uuid.MustNewUUID().String())
	incoming := []network.NetInterface{
		{},
		{},
	}
	s.st.EXPECT().MergeLinkLayerDevice(gomock.Any(), machineUUID.String(), incoming).Return(nil)

	// Act
	err := s.service(c).MergeLinkLayerDevice(c.Context(), machineUUID, incoming)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}
