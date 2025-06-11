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
	"github.com/juju/juju/core/unit"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type netConfigSuite struct {
	testhelpers.IsolationSuite

	st *MockState
}

func (s *netConfigSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = NewMockState(ctrl)
	c.Cleanup(func() { s.st = nil })
	return ctrl
}

func (s *netConfigSuite) service(c *tc.C) *Service {
	return NewService(s.st, loggertesting.WrapCheckLog(c))
}

func TestNetConfigSuite(t *testing.T) {
	tc.Run(t, &netConfigSuite{})
}

func (s *netConfigSuite) TestImportLinkLayerDevices(c *tc.C) {
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

func (s *netConfigSuite) TestImportLinkLayerDevicesMachines(c *tc.C) {
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

func (s *netConfigSuite) TestImportLinkLayerDevicesNoContent(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), []internal.ImportLinkLayerDevice{})

	// Assert: no failure if no data provided.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *netConfigSuite) TestDeleteImportedLinkLayerDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.st.EXPECT().DeleteImportedLinkLayerDevices(gomock.Any()).Return(errors.New("boom"))

	// Act
	err := s.migrationService(c).DeleteImportedLinkLayerDevices(c.Context())

	// Assert: the error from DeleteImportedLinkLayerDevices is passed
	// through to the caller.
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *netConfigSuite) migrationService(c *tc.C) *MigrationService {
	return NewMigrationService(s.st, loggertesting.WrapCheckLog(c))
}

func (s *netConfigSuite) TestSetMachineNetConfigBadUUIDError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machine.UUID("bad-machine-uuid")

	err := s.service(c).SetMachineNetConfig(c.Context(), mUUID, nil)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *netConfigSuite) TestSetMachineNetConfigNodeNotFoundError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID, err := machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), mUUID.String()).Return("", machineerrors.MachineNotFound)

	nics := []network.NetInterface{{Name: "eth0"}}

	err = s.service(c).SetMachineNetConfig(c.Context(), mUUID, nics)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *netConfigSuite) TestSetMachineNetConfigSetCallError(c *tc.C) {
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

func (s *netConfigSuite) TestSetMachineNetConfigEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID, err := machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.service(c).SetMachineNetConfig(c.Context(), mUUID, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *netConfigSuite) TestSetMachineNetConfig(c *tc.C) {
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

func (s *netConfigSuite) TestSetProviderNetConfigInvalidMachineUUID(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	invalidUUID := machine.UUID("invalid-uuid")

	// Act
	err := s.service(c).SetProviderNetConfig(c.Context(), invalidUUID, nil)

	// Assert
	c.Assert(err, tc.ErrorMatches, `invalid machine UUID: id "invalid-uuid" not valid`)
}

func (s *netConfigSuite) TestSetProviderNetConfigGetNetNodeError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	machineUUID := machine.UUID(uuid.MustNewUUID().String())
	stateErr := errors.New("boom")

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).Return("", stateErr)

	// Act
	err := s.service(c).SetProviderNetConfig(c.Context(), machineUUID, nil)

	// Assert
	c.Assert(err, tc.ErrorIs, stateErr)
}

func (s *netConfigSuite) TestSetProviderNetConfigError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	machineUUID := machine.UUID(uuid.MustNewUUID().String())
	nodeUUID := "node-uuid"
	incoming := []network.NetInterface{{}, {}}
	stateErr := errors.New("boom")

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).Return(nodeUUID, nil)
	s.st.EXPECT().MergeLinkLayerDevice(gomock.Any(), nodeUUID, incoming).Return(stateErr)

	// Act
	err := s.service(c).SetProviderNetConfig(c.Context(), machineUUID, incoming)

	// Assert
	c.Assert(err, tc.ErrorIs, stateErr)
}

func (s *netConfigSuite) TestSetProviderNetConfig(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	machineUUID := machine.UUID(uuid.MustNewUUID().String())
	nodeUUID := "node-uuid"
	incoming := []network.NetInterface{
		{},
		{},
	}
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).Return(nodeUUID, nil)
	s.st.EXPECT().MergeLinkLayerDevice(gomock.Any(), nodeUUID, incoming).Return(nil)

	// Act
	err := s.service(c).SetProviderNetConfig(c.Context(), machineUUID, incoming)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *netConfigSuite) TestGetPublicAddressUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID(""), errors.New("boom"))

	_, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *netConfigSuite) TestGetPublicAddressWithCloudServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(nil, errors.New("boom"))

	_, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *netConfigSuite) TestGetPublicAddressNonMatchingAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	nonMatchingScopeAddrs := corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.1.1",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.1.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("foo/0")).Return(unit.UUID("foo-uuid"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo-uuid")).Return(nonMatchingScopeAddrs, nil)

	_, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, "no public address.*")
}

func (s *netConfigSuite) TestGetPublicAddressMatchingAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	matchingScopeAddrs := corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopePublic,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.3",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeCloudLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(matchingScopeAddrs, nil)

	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// Since the second address is higher in hierarchy of scope match, it should
	// be returned.
	c.Check(addr, tc.DeepEquals, matchingScopeAddrs[1])
}

func (s *netConfigSuite) TestGetPublicAddressMatchingAddressSameOrigin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	matchingScopeAddrs := corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.AlphaSpaceId,
			Origin:  corenetwork.OriginProvider,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			Origin:  corenetwork.OriginProvider,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopePublic,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			Origin:  corenetwork.OriginProvider,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.3",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopePublic,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(matchingScopeAddrs, nil)

	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// Since the second address is higher in hierarchy of scope match, it should
	// be returned.
	c.Check(addr, tc.DeepEquals, matchingScopeAddrs[1])
}

func (s *netConfigSuite) TestGetPublicAddressMatchingAddressOneProviderOnly(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	matchingScopeAddrs := corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.AlphaSpaceId,
			Origin:  corenetwork.OriginMachine,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			Origin:  corenetwork.OriginMachine,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopePublic,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			Origin:  corenetwork.OriginProvider,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.3",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopePublic,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(matchingScopeAddrs, nil)

	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// Since the second address is higher in hierarchy of scope match, it should
	// be returned.
	c.Check(addr, tc.DeepEquals, matchingScopeAddrs[2])
}

func (s *netConfigSuite) TestGetPublicAddressMatchingAddressOneProviderOtherUnknown(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	matchingScopeAddrs := corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.AlphaSpaceId,
			Origin:  corenetwork.OriginMachine,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			Origin:  corenetwork.OriginUnknown,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopePublic,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			Origin:  corenetwork.OriginProvider,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.3",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopePublic,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(matchingScopeAddrs, nil)

	addr, err := s.service(c).GetUnitPublicAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// Since the second address is higher in hierarchy of scope match, it should
	// be returned.
	c.Check(addr, tc.DeepEquals, matchingScopeAddrs[2])
}

func (s *netConfigSuite) TestGetPublicAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	unitAddresses := corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopePublic,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopePublic,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.3",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeCloudLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(unitAddresses, nil)

	addrs, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// The two public addresses should be returned.
	c.Check(addrs, tc.DeepEquals, unitAddresses[0:2])
}

func (s *netConfigSuite) TestGetPublicAddressesCloudLocal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	unitAddresses := corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeCloudLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeCloudLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.3",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(unitAddresses, nil)

	addrs, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// The two cloud-local addresses should be returned because there are no
	// public ones.
	c.Check(addrs, tc.DeepEquals, unitAddresses[0:2])
}

func (s *netConfigSuite) TestGetPublicAddressesNoAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAndK8sServiceAddresses(gomock.Any(), unit.UUID("foo")).Return(corenetwork.SpaceAddresses{}, nil)

	_, err := s.service(c).GetUnitPublicAddresses(c.Context(), unitName)
	c.Assert(err, tc.Satisfies, corenetwork.IsNoAddressError)
}

func (s *netConfigSuite) TestGetPrivateAddressUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), errors.New("boom"))

	_, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *netConfigSuite) TestGetPrivateAddressError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAddresses(gomock.Any(), unit.UUID("foo")).Return(nil, errors.New("boom"))

	_, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *netConfigSuite) TestGetPrivateAddressNonMatchingAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	nonMatchingScopeAddrs := corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.1",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.1.1",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.1.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("foo/0")).Return(unit.UUID("foo-uuid"), nil)
	s.st.EXPECT().GetUnitAddresses(gomock.Any(), unit.UUID("foo-uuid")).Return(nonMatchingScopeAddrs, nil)

	addr, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// We always return the (first) container address even if it doesn't match
	// the scope.
	c.Assert(addr, tc.DeepEquals, nonMatchingScopeAddrs[0])
}

func (s *netConfigSuite) TestGetPrivateAddressMatchingAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	matchingScopeAddrs := corenetwork.SpaceAddresses{
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "54.32.1.2",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopePublic,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "192.168.1.2",
				ConfigType: corenetwork.ConfigStatic,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeCloudLocal,
			},
		},
		{
			SpaceID: corenetwork.AlphaSpaceId,
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.2",
				ConfigType: corenetwork.ConfigDHCP,
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeMachineLocal,
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("foo/0")).Return(unit.UUID("foo-uuid"), nil)
	s.st.EXPECT().GetUnitAddresses(gomock.Any(), unit.UUID("foo-uuid")).Return(matchingScopeAddrs, nil)

	addrs, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	// Since the second address is higher in hierarchy of scope match, it should
	// be returned.
	c.Check(addrs, tc.DeepEquals, matchingScopeAddrs[1])
}

func (s *netConfigSuite) TestGetUnitPrivateAddressNoAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("foo/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unit.UUID("foo"), nil)
	s.st.EXPECT().GetUnitAddresses(gomock.Any(), unit.UUID("foo")).Return(corenetwork.SpaceAddresses{}, nil)

	_, err := s.service(c).GetUnitPrivateAddress(c.Context(), unitName)
	c.Assert(err, tc.Satisfies, corenetwork.IsNoAddressError)
}
