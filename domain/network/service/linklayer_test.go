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

func (s *linkLayerSuite) TestSetMachineNetConfigBadUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machine.UUID("bad-machine-uuid")

	err := NewService(s.st, loggertesting.WrapCheckLog(c)).SetMachineNetConfig(c.Context(), mUUID, nil)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *linkLayerSuite) TestSetMachineNetConfigEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID, err := machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = NewService(s.st, loggertesting.WrapCheckLog(c)).SetMachineNetConfig(c.Context(), mUUID, nil)
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
	exp.SetMachineNetConfig(gomock.Any(), nUUID, nics)

	err = NewService(s.st, loggertesting.WrapCheckLog(c)).SetMachineNetConfig(ctx, mUUID, nics)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *linkLayerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = NewMockState(ctrl)
	c.Cleanup(func() { s.st = nil })
	return ctrl
}
