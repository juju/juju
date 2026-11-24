// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	networkerrors "github.com/juju/juju/domain/network/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *stateSuite) TestUpsertSubnets(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "provider-space-id-1", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	subnetsToUpsert := []network.SubnetInfo{
		{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			AvailabilityZones: []string{"az0"},
		},
		{
			ID:                network.Id(subnetUUID1.String()),
			CIDR:              "10.0.0.0/12",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id-1",
			AvailabilityZones: []string{"az1"},
			SpaceID:           spUUID,
		},
	}
	err = st.UpsertSubnets(c.Context(), subnetsToUpsert)
	c.Assert(err, tc.ErrorIsNil)

	sn1, err := st.GetSubnet(c.Context(), subnetUUID1.String())
	c.Assert(err, tc.ErrorIsNil)
	expected := &network.SubnetInfo{
		ID:                network.Id(subnetUUID1.String()),
		CIDR:              "10.0.0.0/12",
		ProviderId:        "provider-id-1",
		ProviderSpaceId:   "provider-space-id-1",
		ProviderNetworkId: "provider-network-id-1",
		VLANTag:           0,
		AvailabilityZones: []string{"az1"},
		SpaceID:           spUUID,
		SpaceName:         "space0",
	}
	c.Check(sn1, tc.DeepEquals, expected)
	expected = &network.SubnetInfo{
		ID:                network.Id(subnetUUID0.String()),
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderNetworkId: "provider-network-id-0",
		VLANTag:           0,
		AvailabilityZones: []string{"az0"},
		SpaceID:           network.AlphaSpaceId, // alpha space by default.
		SpaceName:         network.AlphaSpaceName,
	}
	sn0, err := st.GetSubnet(c.Context(), subnetUUID0.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sn0, tc.DeepEquals, expected)

	// Update the first subnet to space0 through provider discovery.
	updatedSubnetUUID, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	subnetsToUpsert = []network.SubnetInfo{
		{
			ID:                network.Id(updatedSubnetUUID.String()),
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			SpaceID:           spUUID,
		},
	}
	err = st.UpsertSubnets(c.Context(), subnetsToUpsert)
	c.Assert(err, tc.ErrorIsNil)

	expected = &network.SubnetInfo{
		ID:                network.Id(subnetUUID0.String()),
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderSpaceId:   "provider-space-id-1",
		ProviderNetworkId: "provider-network-id-0",
		VLANTag:           0,
		AvailabilityZones: []string{"az0"},
		SpaceID:           spUUID,
		SpaceName:         "space0",
	}
	sn0, err = st.GetSubnet(c.Context(), subnetUUID0.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sn0, tc.DeepEquals, expected)
}

func (s *stateSuite) TestUpsertSubnetsRepeatedDiscovery(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	original := newTestSubnetInfo(c, "10.10.0.0/24", "provider-id", "provider-network-id")
	err := st.UpsertSubnets(c.Context(), []network.SubnetInfo{original})
	c.Assert(err, tc.ErrorIsNil)

	rediscovered := original
	rediscovered.ID = newSubnetIDForTest(c)
	err = st.UpsertSubnets(c.Context(), []network.SubnetInfo{rediscovered})
	c.Assert(err, tc.ErrorIsNil)

	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 1)
	c.Check(subnets[0].ID, tc.Equals, original.ID)
	c.Check(subnets[0].ProviderId, tc.Equals, original.ProviderId)
}

func (s *stateSuite) TestUpsertSubnetsEmptySpacePreservesExisting(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spaceUUID := networktesting.GenSpaceUUID(c)
	c.Assert(st.AddSpace(c.Context(), spaceUUID, "custom-space", "provider-space-id", nil), tc.ErrorIsNil)

	tests := []struct {
		name              string
		cidr              string
		providerID        network.Id
		providerNetworkID network.Id
	}{
		{
			name:              "provider-id-match",
			cidr:              "10.11.0.0/24",
			providerID:        "provider-a",
			providerNetworkID: "network-a",
		},
		{
			name:              "cidr-match",
			cidr:              "10.11.1.0/24",
			providerNetworkID: "network-b",
		},
	}

	for _, test := range tests {
		existing := newTestSubnetInfo(c, test.cidr, test.providerID, test.providerNetworkID)
		existing.SpaceID = spaceUUID
		err := st.UpsertSubnets(c.Context(), []network.SubnetInfo{existing})
		c.Assert(err, tc.ErrorIsNil)

		rediscovered := existing
		rediscovered.ID = newSubnetIDForTest(c)
		rediscovered.SpaceID = ""
		err = st.UpsertSubnets(c.Context(), []network.SubnetInfo{rediscovered})
		c.Assert(err, tc.ErrorIsNil)

		persisted, err := st.GetSubnet(c.Context(), existing.ID.String())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(persisted.SpaceID, tc.Equals, spaceUUID)
	}
}

func (s *stateSuite) TestUpsertSubnetsEmptySpaceMixedBatch(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spaceUUID := networktesting.GenSpaceUUID(c)
	c.Assert(st.AddSpace(c.Context(), spaceUUID, "custom-space", "provider-space-id", nil), tc.ErrorIsNil)

	existing := newTestSubnetInfo(c, "10.12.0.0/24", "existing-provider", "existing-network")
	existing.SpaceID = spaceUUID
	c.Assert(st.UpsertSubnets(c.Context(), []network.SubnetInfo{existing}), tc.ErrorIsNil)

	rediscovered := existing
	rediscovered.ID = newSubnetIDForTest(c)
	rediscovered.SpaceID = ""
	addition := newTestSubnetInfo(c, "10.12.1.0/24", "new-provider", "new-network")
	addition.SpaceID = ""
	c.Assert(st.UpsertSubnets(c.Context(), []network.SubnetInfo{rediscovered, addition}), tc.ErrorIsNil)

	persistedExisting, err := st.GetSubnet(c.Context(), existing.ID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(persistedExisting.SpaceID, tc.Equals, spaceUUID)
	persistedAddition, err := st.GetSubnet(c.Context(), addition.ID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(persistedAddition.SpaceID, tc.Equals, network.AlphaSpaceId)
}

func (s *stateSuite) TestUpsertSubnetsExplicitAlphaSpace(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spaceUUID := networktesting.GenSpaceUUID(c)
	c.Assert(st.AddSpace(c.Context(), spaceUUID, "custom-space", "provider-space-id", nil), tc.ErrorIsNil)

	existing := newTestSubnetInfo(c, "10.13.0.0/24", "provider-id", "provider-network-id")
	existing.SpaceID = spaceUUID
	c.Assert(st.UpsertSubnets(c.Context(), []network.SubnetInfo{existing}), tc.ErrorIsNil)

	rediscovered := existing
	rediscovered.ID = newSubnetIDForTest(c)
	rediscovered.SpaceID = network.AlphaSpaceId
	c.Assert(st.UpsertSubnets(c.Context(), []network.SubnetInfo{rediscovered}), tc.ErrorIsNil)

	persisted, err := st.GetSubnet(c.Context(), existing.ID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(persisted.SpaceID, tc.Equals, network.AlphaSpaceId)
}

func (s *stateSuite) TestUpsertSubnetsExistingAndNew(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	tests := []struct {
		name          string
		existingCIDR  string
		newCIDR       string
		existingFirst bool
	}{
		{
			name:          "existing-first",
			existingCIDR:  "10.20.0.0/24",
			newCIDR:       "10.20.1.0/24",
			existingFirst: true,
		},
		{
			name:          "new-first",
			existingCIDR:  "10.20.2.0/24",
			newCIDR:       "10.20.3.0/24",
			existingFirst: false,
		},
	}

	for _, test := range tests {
		existing := newTestSubnetInfo(
			c, test.existingCIDR,
			network.Id(test.name+"-existing-provider"),
			network.Id(test.name+"-existing-network"),
		)
		err := st.UpsertSubnets(c.Context(), []network.SubnetInfo{existing})
		c.Assert(err, tc.ErrorIsNil)

		rediscovered := existing
		rediscovered.ID = newSubnetIDForTest(c)
		addition := newTestSubnetInfo(
			c, test.newCIDR,
			network.Id(test.name+"-new-provider"),
			network.Id(test.name+"-new-network"),
		)
		batch := []network.SubnetInfo{addition, rediscovered}
		if test.existingFirst {
			batch = []network.SubnetInfo{rediscovered, addition}
		}

		err = st.UpsertSubnets(c.Context(), batch)
		c.Assert(err, tc.ErrorIsNil)

		persistedExisting, err := st.GetSubnet(c.Context(), existing.ID.String())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(persistedExisting.CIDR, tc.Equals, existing.CIDR)
		persistedAddition, err := st.GetSubnet(c.Context(), addition.ID.String())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(persistedAddition.CIDR, tc.Equals, addition.CIDR)
	}
}

func (s *stateSuite) TestUpsertSubnetsProviderIDCIDRConflictRollsBack(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	existing := newTestSubnetInfo(c, "10.30.0.0/24", "existing-provider", "existing-network")
	err := st.UpsertSubnets(c.Context(), []network.SubnetInfo{existing})
	c.Assert(err, tc.ErrorIsNil)

	addition := newTestSubnetInfo(c, "10.30.1.0/24", "new-provider", "new-network")
	conflict := newTestSubnetInfo(c, "10.30.99.0/24", existing.ProviderId, existing.ProviderNetworkId)
	err = st.UpsertSubnets(c.Context(), []network.SubnetInfo{addition, conflict})
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
	c.Check(err, tc.ErrorMatches, `provider id "existing-provider" is associated with CIDR "10.30.0.0/24", not "10.30.99.0/24"`)

	_, err = st.GetSubnet(c.Context(), addition.ID.String())
	c.Assert(err, tc.ErrorIs, networkerrors.SubnetNotFound)
	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.HasLen, 1)
}

func (s *stateSuite) TestUpsertSubnetsEmptyProviderID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	original := newTestSubnetInfo(c, "0.0.0.0/0", "", "")
	err := st.UpsertSubnets(c.Context(), []network.SubnetInfo{original})
	c.Assert(err, tc.ErrorIsNil)

	rediscovered := original
	rediscovered.ID = newSubnetIDForTest(c)
	err = st.UpsertSubnets(c.Context(), []network.SubnetInfo{rediscovered})
	c.Assert(err, tc.ErrorIsNil)

	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 1)
	c.Check(subnets[0].ID, tc.Equals, original.ID)
	s.assertNumProviderSubnets(c, s.DB(), original.ID.String(), 0)
}

func (s *stateSuite) TestUpsertSubnetsAddsProviderIDToExisting(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	original := newTestSubnetInfo(c, "10.40.0.0/24", "", "provider-network-id")
	err := st.UpsertSubnets(c.Context(), []network.SubnetInfo{original})
	c.Assert(err, tc.ErrorIsNil)

	rediscovered := original
	rediscovered.ID = newSubnetIDForTest(c)
	rediscovered.ProviderId = "new-provider-id"
	err = st.UpsertSubnets(c.Context(), []network.SubnetInfo{rediscovered})
	c.Assert(err, tc.ErrorIsNil)

	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 1)
	c.Check(subnets[0].ID, tc.Equals, original.ID)
	c.Check(subnets[0].ProviderId, tc.Equals, rediscovered.ProviderId)
	s.assertProviderSubnetProviderID(c, s.DB(), original.ID.String(), rediscovered.ProviderId.String())
}

func (s *stateSuite) TestUpsertSubnetsBackfillsProviderAssociations(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spaceUUID := networktesting.GenSpaceUUID(c)
	c.Assert(st.AddSpace(c.Context(), spaceUUID, "custom-space", "provider-space-id", nil), tc.ErrorIsNil)

	original := newTestSubnetInfo(c, "10.41.0.0/24", "", "")
	original.SpaceID = spaceUUID
	c.Assert(st.UpsertSubnets(c.Context(), []network.SubnetInfo{original}), tc.ErrorIsNil)

	rediscovered := original
	rediscovered.ID = newSubnetIDForTest(c)
	rediscovered.ProviderId = "new-provider-id"
	rediscovered.ProviderNetworkId = "new-provider-network-id"
	rediscovered.SpaceID = ""
	c.Assert(st.UpsertSubnets(c.Context(), []network.SubnetInfo{rediscovered}), tc.ErrorIsNil)

	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 1)
	c.Check(subnets[0].ID, tc.Equals, original.ID)
	c.Check(subnets[0].ProviderId, tc.Equals, rediscovered.ProviderId)
	c.Check(subnets[0].ProviderNetworkId, tc.Equals, rediscovered.ProviderNetworkId)
	c.Check(subnets[0].SpaceID, tc.Equals, spaceUUID)
	s.assertProviderSubnetProviderID(c, s.DB(), original.ID.String(), rediscovered.ProviderId.String())
	s.assertNumProviderNetworkSubnets(c, s.DB(), original.ID.String(), 1)
}

func (s *stateSuite) TestUpsertSubnetsIncomingProviderNetworkIDEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	original := newTestSubnetInfo(c, "10.42.0.0/24", "", "provider-network-id")
	c.Assert(st.UpsertSubnets(c.Context(), []network.SubnetInfo{original}), tc.ErrorIsNil)

	rediscovered := original
	rediscovered.ID = newSubnetIDForTest(c)
	rediscovered.ProviderNetworkId = ""
	c.Assert(st.UpsertSubnets(c.Context(), []network.SubnetInfo{rediscovered}), tc.ErrorIsNil)

	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 1)
	c.Check(subnets[0].ID, tc.Equals, original.ID)
	c.Check(subnets[0].ProviderNetworkId, tc.Equals, original.ProviderNetworkId)
}

func (s *stateSuite) TestUpsertSubnetsIncomingProviderIDEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	original := newTestSubnetInfo(c, "10.43.0.0/24", "provider-id", "provider-network-id")
	c.Assert(st.UpsertSubnets(c.Context(), []network.SubnetInfo{original}), tc.ErrorIsNil)

	rediscovered := original
	rediscovered.ID = newSubnetIDForTest(c)
	rediscovered.ProviderId = ""
	c.Assert(st.UpsertSubnets(c.Context(), []network.SubnetInfo{rediscovered}), tc.ErrorIsNil)

	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 1)
	c.Check(subnets[0].ID, tc.Equals, original.ID)
	c.Check(subnets[0].ProviderId, tc.Equals, original.ProviderId)
	s.assertProviderSubnetProviderID(c, s.DB(), original.ID.String(), original.ProviderId.String())
}

func (s *stateSuite) TestUpsertSubnetsSameCIDRDifferentProviderNetworks(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	first := newTestSubnetInfo(c, "10.50.0.0/24", "provider-a", "network-a")
	second := newTestSubnetInfo(c, first.CIDR, "provider-b", "network-b")
	err := st.UpsertSubnets(c.Context(), []network.SubnetInfo{first, second})
	c.Assert(err, tc.ErrorIsNil)

	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.HasLen, 2)
}

func (s *stateSuite) TestUpsertSubnetsConflictingProviderIDs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	existing := newTestSubnetInfo(c, "10.60.0.0/24", "provider-a", "network-a")
	err := st.UpsertSubnets(c.Context(), []network.SubnetInfo{existing})
	c.Assert(err, tc.ErrorIsNil)

	conflicting := newTestSubnetInfo(c, existing.CIDR, "provider-b", existing.ProviderNetworkId)
	err = st.UpsertSubnets(c.Context(), []network.SubnetInfo{conflicting})
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
	c.Check(err, tc.ErrorMatches, `provider id "provider-b" does not match existing provider id "provider-a" for CIDR "10.60.0.0/24"`)

	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.HasLen, 1)
}

func (s *stateSuite) TestUpsertSubnetsAmbiguousCIDR(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	first := newTestSubnetInfo(c, "10.70.0.0/24", "", "network-a")
	second := newTestSubnetInfo(c, first.CIDR, "", first.ProviderNetworkId)
	c.Assert(st.AddSubnet(c.Context(), first), tc.ErrorIsNil)
	c.Assert(st.AddSubnet(c.Context(), second), tc.ErrorIsNil)

	incoming := newTestSubnetInfo(c, first.CIDR, "", first.ProviderNetworkId)
	err := st.UpsertSubnets(c.Context(), []network.SubnetInfo{incoming})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `multiple subnets found for CIDR "10.70.0.0/24" and provider network id "network-a"`)

	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.HasLen, 2)
}

func (s *stateSuite) TestUpsertSubnetsDuplicateInput(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	first := newTestSubnetInfo(c, "10.80.0.0/24", "provider-id", "provider-network-id")
	duplicate := first
	duplicate.ID = newSubnetIDForTest(c)
	err := st.UpsertSubnets(c.Context(), []network.SubnetInfo{first, duplicate})
	c.Assert(err, tc.ErrorIsNil)

	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 1)
	c.Check(subnets[0].ID, tc.Equals, first.ID)
}

func (s *stateSuite) TestGetSubnetsByCIDRMultipleAndRepeated(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	first := newTestSubnetInfo(c, "10.90.0.0/24", "provider-a", "network-a")
	first.AvailabilityZones = []string{"az-a", "az-b"}
	second := newTestSubnetInfo(c, "10.90.1.0/24", "provider-b", "network-b")
	second.AvailabilityZones = []string{"az-c"}
	err := st.UpsertSubnets(c.Context(), []network.SubnetInfo{first, second})
	c.Assert(err, tc.ErrorIsNil)

	subnets, err := st.GetSubnetsByCIDR(c.Context(), first.CIDR, second.CIDR, first.CIDR)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 2)

	byCIDR := make(map[string]network.SubnetInfo, len(subnets))
	for _, subnet := range subnets {
		byCIDR[subnet.CIDR] = subnet
	}
	c.Check(byCIDR[first.CIDR].ID, tc.Equals, first.ID)
	c.Check(byCIDR[first.CIDR].AvailabilityZones, tc.SameContents, first.AvailabilityZones)
	c.Check(byCIDR[second.CIDR].ID, tc.Equals, second.ID)
	c.Check(byCIDR[second.CIDR].AvailabilityZones, tc.SameContents, second.AvailabilityZones)
}

func (s *stateSuite) TestAddSubnet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(uuid.String()),
			CIDR:              "10.0.0.0/24",
			ProviderId:        "provider-id",
			ProviderNetworkId: "provider-network-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Check the subnet entity.
	s.assertSubnet(c, db, uuid.String(), "10.0.0.0/24", 0, spUUID.String())

	// Check the provider network entity.
	s.assertProviderNetworkID(c, db, uuid.String(), "provider-network-id")

	// Check the provider subnet entity.
	s.assertProviderSubnetProviderID(c, db, uuid.String(), "provider-id")

	// Check the az entity.
	s.assertSubnetAZs(c, db, uuid.String(), []string{"az0", "az1"})
}

func (s *stateSuite) TestAddSubnetNoProviderID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(uuid.String()),
			CIDR:              "10.0.0.0/24",
			ProviderNetworkId: "provider-network-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Check the subnet entity.
	s.assertSubnet(c, db, uuid.String(), "10.0.0.0/24", 0, spUUID.String())

	// Check the provider network entity.
	s.assertProviderNetworkID(c, db, uuid.String(), "provider-network-id")

	// Check the provider subnet entity.
	s.assertNumProviderSubnets(c, db, uuid.String(), 0)

	// Check the az entity.
	s.assertSubnetAZs(c, db, uuid.String(), []string{"az0", "az1"})
}

func (s *stateSuite) TestAddSubnetNoProviderNetworkID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(uuid.String()),
			CIDR:              "10.0.0.0/24",
			ProviderId:        "provider-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Check the subnet entity.
	s.assertSubnet(c, db, uuid.String(), "10.0.0.0/24", 0, spUUID.String())

	// Check the provider network entity.
	s.assertNumProviderNetworkID(c, db, uuid.String(), 0)
	s.assertNumProviderNetworkSubnets(c, db, uuid.String(), 0)

	// Check the provider subnet entity.
	s.assertProviderSubnetProviderID(c, db, uuid.String(), "provider-id")

	// Check the az entity.
	s.assertSubnetAZs(c, db, uuid.String(), []string{"az0", "az1"})
}

func (s *stateSuite) TestAddSubnetNoProviderIDs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(uuid.String()),
			CIDR:              "10.0.0.0/24",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Check the subnet entity.
	s.assertSubnet(c, db, uuid.String(), "10.0.0.0/24", 0, spUUID.String())

	// Check the provider network entity.
	s.assertNumProviderNetworkID(c, db, uuid.String(), 0)
	s.assertNumProviderNetworkSubnets(c, db, uuid.String(), 0)

	// Check the provider subnet entity.
	s.assertNumProviderSubnets(c, db, uuid.String(), 0)

	// Check the az entity.
	s.assertSubnetAZs(c, db, uuid.String(), []string{"az0", "az1"})
}

func (s *stateSuite) TestAddSubnetsSameNetworkID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "10.0.0.0/24",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID1.String()),
			CIDR:              "10.0.1.0/24",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	s.assertSubnet(c, db, subnetUUID0.String(), "10.0.0.0/24", 0, spUUID.String())
	s.assertSubnet(c, db, subnetUUID1.String(), "10.0.1.0/24", 0, spUUID.String())
}

func (s *stateSuite) TestFailAddTwoSubnetsSameProviderID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "10.0.0.0/24",
			ProviderId:        "provider-id",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID1.String()),
			CIDR:              "10.0.1.0/24",
			ProviderId:        "provider-id",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		},
	)
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
}

func (s *stateSuite) TestRetrieveFanSubnet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add a subnet of type base.
	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Add a subnet of type fan.
	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID1.String()),
			CIDR:              "10.0.0.0/12",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{"az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	expected := network.SubnetInfo{
		ID:                network.Id(subnetUUID1.String()),
		CIDR:              "10.0.0.0/12",
		ProviderId:        "provider-id-1",
		ProviderNetworkId: "provider-network-id-1",
		VLANTag:           0,
		AvailabilityZones: []string{"az1"},
		SpaceID:           network.AlphaSpaceId, // alpha space by default.
		SpaceName:         network.AlphaSpaceName,
	}

	// Get the fan subnet by uuid.
	sn1, err := st.GetSubnet(c.Context(), subnetUUID1.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sn1, tc.DeepEquals, &expected)

	// Get the fan subnet by cidr.
	subnetsByCIDR, err := st.GetSubnetsByCIDR(c.Context(), "10.0.0.0/12")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnetsByCIDR, tc.HasLen, 1)
	c.Check(subnetsByCIDR[0], tc.DeepEquals, expected)

	// Get all subnets.
	allExpected := network.SubnetInfos{
		expected,
		{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0"},
			SpaceID:           network.AlphaSpaceId, // alpha space by default.
			SpaceName:         network.AlphaSpaceName,
		},
	}
	allSubnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(allSubnets, tc.HasLen, 2)
	c.Check(allSubnets, tc.SameContents, allExpected)
}

func (s *stateSuite) TestRetrieveSubnetByUUID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add a subnet of type base.
	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Add a space with subnet base.
	spUUID := networktesting.GenSpaceUUID(c)
	err = st.AddSpace(c.Context(), spUUID, "space0", "provider-space-id", []string{"192.168.0.0/20"})
	c.Assert(err, tc.ErrorIsNil)

	expected := &network.SubnetInfo{
		ID:                network.Id(subnetUUID0.String()),
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderSpaceId:   "provider-space-id",
		ProviderNetworkId: "provider-network-id-0",
		VLANTag:           0,
		AvailabilityZones: []string{"az0"},
		SpaceID:           spUUID,
		SpaceName:         "space0",
	}
	sn0, err := st.GetSubnet(c.Context(), subnetUUID0.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sn0, tc.DeepEquals, expected)
}

func (s *stateSuite) TestRetrieveAllSubnets(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add 3 subnets of type base.
	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID1.String()),
			CIDR:              "192.168.1.0/20",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID2, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID2.String()),
			CIDR:              "192.168.2.0/20",
			ProviderId:        "provider-id-2",
			ProviderNetworkId: "provider-network-id-2",
			VLANTag:           0,
			AvailabilityZones: []string{"az2", "az3"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	sns, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sns, tc.HasLen, 3)
}

func (s *stateSuite) TestRetrieveAllSubnet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add 3 subnets of type base.
	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "192.168.0.0/20",
			ProviderId:        "0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID1.String()),
			CIDR:              "192.168.1.0/20",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID2, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID2.String()),
			CIDR:              "192.168.2.0/20",
			ProviderId:        "provider-id-2",
			ProviderNetworkId: "provider-network-id-2",
			VLANTag:           0,
			AvailabilityZones: []string{"az2", "az3"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	sns, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sns, tc.HasLen, 3)
}

func (s *stateSuite) TestUpdateSubnet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID.String()),
			CIDR:              "10.0.0.0/24",
			ProviderId:        "provider-id",
			ProviderNetworkId: "provider-network-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	newSpIUUID := networktesting.GenSpaceUUID(c)
	err = st.AddSpace(c.Context(), newSpIUUID, "space1", "bar", []string{})
	c.Assert(err, tc.ErrorIsNil)

	err = st.UpdateSubnet(c.Context(), subnetUUID.String(), newSpIUUID)
	c.Assert(err, tc.ErrorIsNil)

	row := db.QueryRow("SELECT space_uuid FROM subnet WHERE subnet.uuid = ?", subnetUUID.String())
	c.Assert(row.Err(), tc.ErrorIsNil)
	var (
		retrievedSpaceUUID string
	)
	err = row.Scan(&retrievedSpaceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedSpaceUUID, tc.Equals, newSpIUUID.String())
}

func (s *stateSuite) TestDeleteSubnet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add a subnet of type base.
	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	// Add a subnet of type fan.
	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID1.String()),
			CIDR:              "10.0.0.0/12",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	// Add another subnet of type fan.
	subnetUUID2, err := uuid.NewV7()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID2.String()),
			CIDR:              "10.8.0.0/12",
			ProviderId:        "provider-id-2",
			ProviderNetworkId: "provider-network-id-2",
			VLANTag:           0,
			AvailabilityZones: []string{"az4", "az5"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteSubnet(c.Context(), subnetUUID0.String())
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.GetSubnet(c.Context(), subnetUUID0.String())
	c.Assert(err, tc.ErrorIs, networkerrors.SubnetNotFound)
	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.HasLen, 2)

	err = st.DeleteSubnet(c.Context(), subnetUUID1.String())
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.GetSubnet(c.Context(), subnetUUID1.String())
	c.Assert(err, tc.ErrorIs, networkerrors.SubnetNotFound)
	subnets, err = st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.HasLen, 1)

	err = st.DeleteSubnet(c.Context(), subnetUUID2.String())
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.GetSubnet(c.Context(), subnetUUID2.String())
	c.Assert(err, tc.ErrorIs, networkerrors.SubnetNotFound)
	subnets, err = st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.HasLen, 0)
}

// TestCannotAddEmptyProviderSubnet tests that an empty string provider ID
// cannot be added to the provider_subnet table.
func (s *stateSuite) TestCannotAddEmptyProviderSubnet(c *tc.C) {
	// Act
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertSpace := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err := tx.ExecContext(ctx, insertSpace, "space0-uuid", "testing")
		if err != nil {
			return err
		}
		insertSubnet := `INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertSubnet, "subnet-uuid", "10.0.0.0/24", "space0-uuid")
		if err != nil {
			return err
		}
		insertSubnetProvider := `INSERT INTO provider_subnet (provider_id, subnet_uuid) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertSubnetProvider, "", "subnet-uuid")
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorMatches, "CHECK constraint failed: chk_provider_id_empty")
}

func newTestSubnetInfo(
	c *tc.C,
	cidr string,
	providerID network.Id,
	providerNetworkID network.Id,
) network.SubnetInfo {
	return network.SubnetInfo{
		ID:                newSubnetIDForTest(c),
		CIDR:              cidr,
		ProviderId:        providerID,
		ProviderNetworkId: providerNetworkID,
		SpaceID:           network.AlphaSpaceId,
	}
}

func newSubnetIDForTest(c *tc.C) network.Id {
	return network.Id(tc.Must(c, uuid.NewV7).String())
}

func (s *stateSuite) assertSubnet(c *tc.C, db *sql.DB, subnetUUID, expectedCIDR string, expectedVLANTag int, expectedSpaceUUID string) {
	row := db.QueryRow("SELECT cidr, vlan_tag, space_uuid FROM subnet WHERE uuid = ?", subnetUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)
	var (
		cidr, spaceUUID string
		VLANTag         int
	)
	err := row.Scan(&cidr, &VLANTag, &spaceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cidr, tc.Equals, expectedCIDR)
	c.Check(VLANTag, tc.Equals, expectedVLANTag)
	c.Check(spaceUUID, tc.Equals, expectedSpaceUUID)
}

func (s *stateSuite) assertProviderNetworkID(c *tc.C, db *sql.DB, subnetUUID, expectedProviderNetworkID string) {
	row := db.QueryRow(`
SELECT uuid, provider_network_id
FROM provider_network
WHERE provider_network_id = ?`,
		"provider-network-id",
	)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var retrievedProviderNetworkUUID, retrievedProviderNetworkID string
	err := row.Scan(&retrievedProviderNetworkUUID, &retrievedProviderNetworkID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedProviderNetworkID, tc.Equals, expectedProviderNetworkID)

	row = db.QueryRow(`
SELECT subnet_uuid
FROM provider_network_subnet
WHERE provider_network_uuid = ?`,
		retrievedProviderNetworkUUID,
	)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var retrievedSubnetUUID string
	err = row.Scan(&retrievedSubnetUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedSubnetUUID, tc.Equals, subnetUUID)
}

func (s *stateSuite) assertNumProviderNetworkID(c *tc.C, db *sql.DB, subnetUUID string, num int) {
	row := db.QueryRow(`
SELECT COUNT(*)
FROM provider_network_subnet
WHERE subnet_uuid = ?`,
		subnetUUID,
	)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var numProviderNetworkID int
	err := row.Scan(&numProviderNetworkID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(numProviderNetworkID, tc.Equals, num)
}

func (s *stateSuite) assertNumProviderNetworkSubnets(c *tc.C, db *sql.DB, subnetUUID string, num int) {
	row := db.QueryRow("SELECT COUNT(*) FROM provider_network_subnet WHERE subnet_uuid = ?", subnetUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var numProviderNetworkSubnetID int
	err := row.Scan(&numProviderNetworkSubnetID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(numProviderNetworkSubnetID, tc.Equals, num)
}

func (s *stateSuite) assertProviderSubnetProviderID(c *tc.C, db *sql.DB, subnetUUID, expectedProviderID string) {
	row := db.QueryRow("SELECT provider_id FROM provider_subnet WHERE subnet_uuid = ?", subnetUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var retrievedProviderSubnetID string
	err := row.Scan(&retrievedProviderSubnetID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedProviderSubnetID, tc.Equals, expectedProviderID)
}

func (s *stateSuite) assertNumProviderSubnets(c *tc.C, db *sql.DB, subnetUUID string, num int) {
	row := db.QueryRow("SELECT COUNT(*) FROM provider_subnet WHERE subnet_uuid = ?", subnetUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)
	var numProviderSubnetID int
	err := row.Scan(&numProviderSubnetID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(numProviderSubnetID, tc.Equals, num)
}

func (s *stateSuite) assertSubnetAZs(c *tc.C, db *sql.DB, subnetUUID string, expectedAZs []string) {
	rows, err := db.Query(`
SELECT name 
FROM   availability_zone_subnet 
JOIN   availability_zone
ON     availability_zone_uuid = availability_zone.uuid
WHERE  subnet_uuid = ?`, subnetUUID)
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	var retrievedAZs []string
	for rows.Next() {
		var retrievedAZ string
		err = rows.Scan(&retrievedAZ)
		c.Assert(err, tc.ErrorIsNil)

		retrievedAZs = append(retrievedAZs, retrievedAZ)
	}
	c.Check(retrievedAZs, tc.SameContents, expectedAZs)
}
