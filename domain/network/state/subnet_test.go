// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *stateSuite) TestUpsertSubnets(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spUUID := network.GenSpaceUUID(c)
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

	// Update the first subnet to space0.
	subnetsToUpsert = []network.SubnetInfo{
		{
			ID:      network.Id(subnetUUID0.String()),
			SpaceID: spUUID,
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

func (s *stateSuite) TestAddSubnet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := network.GenSpaceUUID(c)
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
	row := db.QueryRow("SELECT cidr,vlan_tag,space_uuid FROM subnet WHERE uuid = ?", uuid.String())
	c.Assert(row.Err(), tc.ErrorIsNil)
	var (
		cidr, spaceUUID string
		VLANTag         int
	)
	err = row.Scan(&cidr, &VLANTag, &spaceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cidr, tc.Equals, "10.0.0.0/24")
	c.Check(VLANTag, tc.Equals, 0)
	c.Check(spaceUUID, tc.Equals, spUUID.String())

	// Check the provider network entity.
	row = db.QueryRow("SELECT uuid,provider_network_id FROM provider_network WHERE provider_network_id = ?", "provider-network-id")
	c.Assert(row.Err(), tc.ErrorIsNil)
	var (
		retrievedProviderNetworkUUID, retrievedProviderNetworkID string
	)
	err = row.Scan(&retrievedProviderNetworkUUID, &retrievedProviderNetworkID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedProviderNetworkID, tc.Equals, "provider-network-id")
	row = db.QueryRow("SELECT subnet_uuid FROM provider_network_subnet WHERE provider_network_uuid = ?", retrievedProviderNetworkUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)
	var retrievedSubnetUUID string
	err = row.Scan(&retrievedSubnetUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedSubnetUUID, tc.Equals, uuid.String())
	// Check the provider subnet entity.
	row = db.QueryRow("SELECT provider_id FROM provider_subnet WHERE subnet_uuid = ?", uuid.String())
	c.Assert(row.Err(), tc.ErrorIsNil)
	var retrievedProviderSubnetID string
	err = row.Scan(&retrievedProviderSubnetID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedProviderSubnetID, tc.Equals, "provider-id")
	// Check the az entity.
	rows, err := db.Query(`
	SELECT name 
	FROM   availability_zone_subnet 
	JOIN   availability_zone
	ON     availability_zone_uuid = availability_zone.uuid
	WHERE  subnet_uuid = ?`, uuid.String())
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	var retrievedAZs []string
	for rows.Next() {
		var retrievedAZ string
		err = rows.Scan(&retrievedAZ)
		c.Assert(err, tc.ErrorIsNil)
		retrievedAZs = append(retrievedAZs, retrievedAZ)
	}
	c.Check(retrievedAZs, tc.SameContents, []string{"az0", "az1"})
}

func (s *stateSuite) TestAddTwoSubnetsSameNetworkID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spUUID := network.GenSpaceUUID(c)
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
}

func (s *stateSuite) TestFailAddTwoSubnetsSameProviderID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spUUID := network.GenSpaceUUID(c)
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
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("provider id %q for subnet %q already exists", "provider-id", subnetUUID1.String()))
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
	spUUID := network.GenSpaceUUID(c)
	err = st.AddSpace(c.Context(), spUUID, "space0", "provider-space-id", []string{subnetUUID0.String()})
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

	spUUID := network.GenSpaceUUID(c)
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

	newSpIUUID := network.GenSpaceUUID(c)
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
