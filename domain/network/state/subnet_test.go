// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	ctx "context"
	"fmt"

	"github.com/google/uuid"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *stateSuite) TestUpsertSubnets(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spUUID, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spUUID.String(), "space0", "provider-space-id-1", []string{})
	c.Assert(err, jc.ErrorIsNil)

	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	subnetsToUpsert := []domainnetwork.SubnetArg{
		{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID0.String()),
				CIDR:              "192.168.0.0/20",
				ProviderId:        "provider-id-0",
				ProviderNetworkId: "provider-network-id-0",
				AvailabilityZones: []string{"az0"},
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("192.168.0.0/20"),
		},
		{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID1.String()),
				CIDR:              "2001:0DB8::/32",
				ProviderId:        "provider-id-1",
				ProviderNetworkId: "provider-network-id-1",
				AvailabilityZones: []string{"az1"},
				SpaceID:           spUUID.String(),
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("2001:0DB8::/32"),
		},
	}
	err = st.UpsertSubnets(ctx.Background(), subnetsToUpsert)
	c.Assert(err, jc.ErrorIsNil)

	sn1, err := st.GetSubnet(ctx.Background(), subnetUUID1.String())
	c.Assert(err, jc.ErrorIsNil)
	expected := &network.SubnetInfo{
		ID:                network.Id(subnetUUID1.String()),
		CIDR:              "2001:0DB8::/32",
		ProviderId:        "provider-id-1",
		ProviderSpaceId:   "provider-space-id-1",
		ProviderNetworkId: "provider-network-id-1",
		VLANTag:           0,
		AvailabilityZones: []string{"az1"},
		SpaceID:           spUUID.String(),
		SpaceName:         "space0",
	}
	c.Check(sn1, gc.DeepEquals, expected)
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
	sn0, err := st.GetSubnet(ctx.Background(), subnetUUID0.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sn0, gc.DeepEquals, expected)

	// Update the first subnet to space0.
	subnetsToUpsert = []domainnetwork.SubnetArg{
		{
			SubnetInfo: network.SubnetInfo{
				ID:      network.Id(subnetUUID0.String()),
				SpaceID: spUUID.String(),
			},
		},
	}
	err = st.UpsertSubnets(ctx.Background(), subnetsToUpsert)
	c.Assert(err, jc.ErrorIsNil)

	expected = &network.SubnetInfo{
		ID:                network.Id(subnetUUID0.String()),
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderSpaceId:   "provider-space-id-1",
		ProviderNetworkId: "provider-network-id-0",
		VLANTag:           0,
		AvailabilityZones: []string{"az0"},
		SpaceID:           spUUID.String(),
		SpaceName:         "space0",
	}
	sn0, err = st.GetSubnet(ctx.Background(), subnetUUID0.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sn0, gc.DeepEquals, expected)
}

func (s *stateSuite) TestAddSubnet(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spUUID.String(), "space0", "foo", []string{})
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(uuid.String()),
				CIDR:              "2001:0DB8::/32",
				ProviderId:        "provider-id",
				ProviderNetworkId: "provider-network-id",
				VLANTag:           0,
				AvailabilityZones: []string{"az0", "az1"},
				SpaceID:           spUUID.String(),
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("2001:0DB8::/32"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Check the subnet entity.
	row := db.QueryRow("SELECT cidr, start_address_msb,start_address_lsb, end_address_msb, end_address_lsb,vlan_tag,space_uuid FROM subnet WHERE uuid = ?", uuid.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var (
		startMSB, startLSB, endMSB, endLSB internaldatabase.Uint64
		cidr, spaceUUID                    string
		VLANTag                            int
	)
	err = row.Scan(&cidr, &startMSB, &startLSB, &endMSB, &endLSB, &VLANTag, &spaceUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cidr, gc.Equals, "2001:0DB8::/32")
	c.Check(startMSB.UnsignedValue, gc.Equals, uint64(0x20010db800000000))
	c.Check(startLSB.UnsignedValue, gc.Equals, uint64(0))
	c.Check(endMSB.UnsignedValue, gc.Equals, uint64(0x20010db8ffffffff))
	c.Check(endLSB.UnsignedValue, gc.Equals, uint64(0xffffffffffffffff))
	c.Check(VLANTag, gc.Equals, 0)
	c.Check(spaceUUID, gc.Equals, spUUID.String())

	// Check the provider network entity.
	row = db.QueryRow("SELECT uuid,provider_network_id FROM provider_network WHERE provider_network_id = ?", "provider-network-id")
	c.Assert(row.Err(), jc.ErrorIsNil)
	var (
		retrievedProviderNetworkUUID, retrievedProviderNetworkID string
	)
	err = row.Scan(&retrievedProviderNetworkUUID, &retrievedProviderNetworkID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(retrievedProviderNetworkID, gc.Equals, "provider-network-id")
	row = db.QueryRow("SELECT subnet_uuid FROM provider_network_subnet WHERE provider_network_uuid = ?", retrievedProviderNetworkUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)
	var retrievedSubnetUUID string
	err = row.Scan(&retrievedSubnetUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(retrievedSubnetUUID, gc.Equals, uuid.String())
	// Check the provider subnet entity.
	row = db.QueryRow("SELECT provider_id FROM provider_subnet WHERE subnet_uuid = ?", uuid.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var retrievedProviderSubnetID string
	err = row.Scan(&retrievedProviderSubnetID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(retrievedProviderSubnetID, gc.Equals, "provider-id")
	// Check the az entity.
	rows, err := db.Query(`
	SELECT name
	FROM   availability_zone_subnet
	JOIN   availability_zone
	ON     availability_zone_uuid = availability_zone.uuid
	WHERE  subnet_uuid = ?`, uuid.String())
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	var retrievedAZs []string
	for rows.Next() {
		var retrievedAZ string
		err = rows.Scan(&retrievedAZ)
		c.Assert(err, jc.ErrorIsNil)
		retrievedAZs = append(retrievedAZs, retrievedAZ)
	}
	c.Check(retrievedAZs, jc.SameContents, []string{"az0", "az1"})

	sn, err := st.GetSubnet(ctx.Background(), uuid.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sn.AvailabilityZones, jc.SameContents, []string{"az0", "az1"})
	sn.AvailabilityZones = nil
	c.Assert(sn, jc.DeepEquals, &network.SubnetInfo{
		ID:                network.Id(uuid.String()),
		CIDR:              "2001:0DB8::/32",
		ProviderId:        "provider-id",
		ProviderSpaceId:   "foo",
		ProviderNetworkId: "provider-network-id",
		SpaceID:           spaceUUID,
		SpaceName:         "space0",
	})
}

func (s *stateSuite) TestAddTwoSubnetsSameNetworkID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spUUID, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spUUID.String(), "space0", "foo", []string{})
	c.Assert(err, jc.ErrorIsNil)

	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID0.String()),
				CIDR:              "10.0.0.0/24",
				ProviderId:        "provider-id-0",
				ProviderNetworkId: "provider-network-id",
				VLANTag:           0,
				AvailabilityZones: []string{"az0", "az1"},
				SpaceID:           spUUID.String(),
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("10.0.0.0/24"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID1.String()),
				CIDR:              "10.0.1.0/24",
				ProviderId:        "provider-id-1",
				ProviderNetworkId: "provider-network-id",
				VLANTag:           0,
				AvailabilityZones: []string{"az0", "az1"},
				SpaceID:           spUUID.String(),
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("10.0.1.0/24"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestFailAddTwoSubnetsSameProviderID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spUUID, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spUUID.String(), "space0", "foo", []string{})
	c.Assert(err, jc.ErrorIsNil)

	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID0.String()),
				CIDR:              "10.0.0.0/24",
				ProviderId:        "provider-id",
				ProviderNetworkId: "provider-network-id-0",
				VLANTag:           0,
				AvailabilityZones: []string{"az0", "az1"},
				SpaceID:           spUUID.String(),
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("10.0.0.0/24"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID1.String()),
				CIDR:              "10.0.1.0/24",
				ProviderId:        "provider-id",
				ProviderNetworkId: "provider-network-id-1",
				VLANTag:           0,
				AvailabilityZones: []string{"az0", "az1"},
				SpaceID:           spUUID.String(),
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("10.0.1.0/24"),
		},
	)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("provider id %q for subnet %q already exists", "provider-id", subnetUUID1.String()))
}

func (s *stateSuite) TestRetrieveFanSubnet(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add a subnet of type base.
	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID0.String()),
				CIDR:              "192.168.0.0/20",
				ProviderId:        "provider-id-0",
				ProviderNetworkId: "provider-network-id-0",
				VLANTag:           0,
				AvailabilityZones: []string{"az0"},
				SpaceID:           "",
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("192.168.0.0/20"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	// Add a subnet of type fan.
	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID1.String()),
				CIDR:              "10.0.0.0/12",
				ProviderId:        "provider-id-1",
				ProviderNetworkId: "provider-network-id-1",
				VLANTag:           0,
				AvailabilityZones: []string{"az1"},
				SpaceID:           "",
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("10.0.0.0/12"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

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
	sn1, err := st.GetSubnet(ctx.Background(), subnetUUID1.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sn1, gc.DeepEquals, &expected)
	// Get the fan subnet by cidr.
	subnetsByCIDR, err := st.GetSubnetsByCIDR(ctx.Background(), "10.0.0.0/12")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnetsByCIDR, gc.HasLen, 1)
	c.Check(subnetsByCIDR[0], gc.DeepEquals, expected)
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
	allSubnets, err := st.GetAllSubnets(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(allSubnets, gc.HasLen, 2)
	c.Check(allSubnets, jc.SameContents, allExpected)
}

func (s *stateSuite) TestRetrieveSubnetByUUID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add a subnet of type base.
	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID0.String()),
				CIDR:              "192.168.0.0/20",
				ProviderId:        "provider-id-0",
				ProviderNetworkId: "provider-network-id-0",
				VLANTag:           0,
				AvailabilityZones: []string{"az0"},
				SpaceID:           "",
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("192.168.0.0/20"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	// Add a space with subnet base.
	spUUID, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spUUID.String(), "space0", "provider-space-id", []string{subnetUUID0.String()})
	c.Assert(err, jc.ErrorIsNil)

	expected := &network.SubnetInfo{
		ID:                network.Id(subnetUUID0.String()),
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderSpaceId:   "provider-space-id",
		ProviderNetworkId: "provider-network-id-0",
		VLANTag:           0,
		AvailabilityZones: []string{"az0"},
		SpaceID:           spUUID.String(),
		SpaceName:         "space0",
	}
	sn0, err := st.GetSubnet(ctx.Background(), subnetUUID0.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sn0, gc.DeepEquals, expected)
}

func (s *stateSuite) TestRetrieveAllSubnets(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add 3 subnets of type base.
	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID0.String()),
				CIDR:              "192.168.0.0/20",
				ProviderId:        "provider-id-0",
				ProviderNetworkId: "provider-network-id-0",
				VLANTag:           0,
				AvailabilityZones: []string{"az0", "az1"},
				SpaceID:           "",
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("192.168.0.0/20"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID1.String()),
				CIDR:              "2001:0DB8::/32",
				ProviderId:        "provider-id-1",
				ProviderNetworkId: "provider-network-id-1",
				VLANTag:           0,
				AvailabilityZones: []string{"az0", "az1"},
				SpaceID:           "",
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("2001:0DB8::/32"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	subnetUUID2, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID2.String()),
				CIDR:              "10.6.0.0/16",
				ProviderId:        "provider-id-2",
				ProviderNetworkId: "provider-network-id-2",
				VLANTag:           0,
				AvailabilityZones: []string{"az2", "az3"},
				SpaceID:           "",
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("10.6.0.0/16"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	sns, err := st.GetAllSubnets(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sns, gc.HasLen, 3)
}

func (s *stateSuite) TestUpdateSubnet(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spUUID.String(), "space0", "foo", []string{})
	c.Assert(err, jc.ErrorIsNil)

	subnetUUID, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID.String()),
				CIDR:              "10.0.0.0/24",
				ProviderId:        "provider-id",
				ProviderNetworkId: "provider-network-id",
				VLANTag:           0,
				AvailabilityZones: []string{"az0", "az1"},
				SpaceID:           spUUID.String(),
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("10.0.0.0/24"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	newSpIUUID, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), newSpIUUID.String(), "space1", "bar", []string{})
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateSubnet(ctx.Background(), subnetUUID.String(), newSpIUUID.String())
	c.Assert(err, jc.ErrorIsNil)

	row := db.QueryRow("SELECT space_uuid FROM subnet WHERE subnet.uuid = ?", subnetUUID.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var (
		retrievedSpaceUUID string
	)
	err = row.Scan(&retrievedSpaceUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(retrievedSpaceUUID, gc.Equals, newSpIUUID.String())
}

func (s *stateSuite) TestDeleteSubnet(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add a subnet of type base.
	subnetUUID0, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID0.String()),
				CIDR:              "192.168.0.0/20",
				ProviderId:        "provider-id-0",
				ProviderNetworkId: "provider-network-id-0",
				VLANTag:           0,
				AvailabilityZones: []string{"az0", "az1"},
				SpaceID:           "",
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("192.168.0.0/20"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	// Add a subnet of type fan.
	subnetUUID1, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID1.String()),
				CIDR:              "10.0.0.0/12",
				ProviderId:        "provider-id-1",
				ProviderNetworkId: "provider-network-id-1",
				VLANTag:           0,
				AvailabilityZones: []string{},
				SpaceID:           "",
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("10.0.0.0/12"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	// Add another subnet of type fan.
	subnetUUID2, err := uuid.NewV7()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		domainnetwork.SubnetArg{
			SubnetInfo: network.SubnetInfo{
				ID:                network.Id(subnetUUID2.String()),
				CIDR:              "192.160.0.0/12",
				ProviderId:        "provider-id-2",
				ProviderNetworkId: "provider-network-id-2",
				VLANTag:           0,
				AvailabilityZones: []string{"az4", "az5"},
				SpaceID:           "",
			},
			CIDRAddressRange: domainnetwork.MustCIDRAddressRangeFromString("192.160.0.0/12"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = st.DeleteSubnet(ctx.Background(), subnetUUID0.String())
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.GetSubnet(ctx.Background(), subnetUUID0.String())
	c.Assert(err, jc.ErrorIs, networkerrors.SubnetNotFound)
	subnets, err := st.GetAllSubnets(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnets, gc.HasLen, 2)

	err = st.DeleteSubnet(ctx.Background(), subnetUUID1.String())
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.GetSubnet(ctx.Background(), subnetUUID1.String())
	c.Assert(err, jc.ErrorIs, networkerrors.SubnetNotFound)
	subnets, err = st.GetAllSubnets(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnets, gc.HasLen, 1)

	err = st.DeleteSubnet(ctx.Background(), subnetUUID2.String())
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.GetSubnet(ctx.Background(), subnetUUID2.String())
	c.Assert(err, jc.ErrorIs, networkerrors.SubnetNotFound)
	subnets, err = st.GetAllSubnets(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnets, gc.HasLen, 0)
}
