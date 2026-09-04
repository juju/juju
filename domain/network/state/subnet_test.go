// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	domainnetwork "github.com/juju/juju/domain/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *stateSuite) TestUpsertSubnets(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "provider-space-id-1", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID0 := tc.Must(c, domainnetwork.NewSubnetUUID)
	subnetUUID1 := tc.Must(c, domainnetwork.NewSubnetUUID)
	subnetsToUpsert := []domainnetwork.SubnetInfo{
		{
			UUID:              subnetUUID0,
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			AvailabilityZones: []string{"az0"},
		},
		{
			UUID:              subnetUUID1,
			CIDR:              "10.0.0.0/12",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id-1",
			AvailabilityZones: []string{"az1"},
			SpaceID:           spUUID,
		},
	}
	err = st.UpsertSubnets(c.Context(), subnetsToUpsert)
	c.Assert(err, tc.ErrorIsNil)

	sn1, err := s.getSubnet(c, subnetUUID1)
	c.Assert(err, tc.ErrorIsNil)
	expected := domainnetwork.SubnetInfo{
		UUID:              subnetUUID1,
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
	expected = domainnetwork.SubnetInfo{
		UUID:              subnetUUID0,
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderNetworkId: "provider-network-id-0",
		VLANTag:           0,
		AvailabilityZones: []string{"az0"},
		SpaceID:           network.AlphaSpaceId, // alpha space by default.
		SpaceName:         network.AlphaSpaceName,
	}
	sn0, err := s.getSubnet(c, subnetUUID0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sn0, tc.DeepEquals, expected)

	// Update the first subnet to space0.
	subnetsToUpsert = []domainnetwork.SubnetInfo{
		{
			UUID:    subnetUUID0,
			SpaceID: spUUID,
		},
	}
	err = st.UpsertSubnets(c.Context(), subnetsToUpsert)
	c.Assert(err, tc.ErrorIsNil)

	expected = domainnetwork.SubnetInfo{
		UUID:              subnetUUID0,
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderSpaceId:   "provider-space-id-1",
		ProviderNetworkId: "provider-network-id-0",
		VLANTag:           0,
		AvailabilityZones: []string{"az0"},
		SpaceID:           spUUID,
		SpaceName:         "space0",
	}
	sn0, err = s.getSubnet(c, subnetUUID0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sn0, tc.DeepEquals, expected)
}

func (s *stateSuite) TestAddSubnet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID := tc.Must(c, domainnetwork.NewSubnetUUID)
	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID,
			CIDR:              "10.0.0.0/24",
			ProviderId:        "provider-id",
			ProviderNetworkId: "provider-network-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Check the subnet entity.
	s.assertSubnet(c, db, subnetUUID, "10.0.0.0/24", 0, spUUID.String())

	// Check the provider network entity.
	s.assertProviderNetworkID(c, db, subnetUUID, "provider-network-id")

	// Check the provider subnet entity.
	s.assertProviderSubnetProviderID(c, db, subnetUUID, "provider-id")

	// Check the az entity.
	s.assertSubnetAZs(c, db, subnetUUID, []string{"az0", "az1"})
}

func (s *stateSuite) TestAddSubnetNoProviderID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID := tc.Must(c, domainnetwork.NewSubnetUUID)
	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID,
			CIDR:              "10.0.0.0/24",
			ProviderNetworkId: "provider-network-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Check the subnet entity.
	s.assertSubnet(c, db, subnetUUID, "10.0.0.0/24", 0, spUUID.String())

	// Check the provider network entity.
	s.assertProviderNetworkID(c, db, subnetUUID, "provider-network-id")

	// Check the provider subnet entity.
	s.assertNumProviderSubnets(c, db, subnetUUID, 0)

	// Check the az entity.
	s.assertSubnetAZs(c, db, subnetUUID, []string{"az0", "az1"})
}

func (s *stateSuite) TestAddSubnetNoProviderNetworkID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID := tc.Must(c, domainnetwork.NewSubnetUUID)
	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID,
			CIDR:              "10.0.0.0/24",
			ProviderId:        "provider-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Check the subnet entity.
	s.assertSubnet(c, db, subnetUUID, "10.0.0.0/24", 0, spUUID.String())

	// Check the provider network entity.
	s.assertNumProviderNetworkID(c, db, subnetUUID, 0)
	s.assertNumProviderNetworkSubnets(c, db, subnetUUID, 0)

	// Check the provider subnet entity.
	s.assertProviderSubnetProviderID(c, db, subnetUUID, "provider-id")

	// Check the az entity.
	s.assertSubnetAZs(c, db, subnetUUID, []string{"az0", "az1"})
}

func (s *stateSuite) TestAddSubnetNoProviderIDs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID := tc.Must(c, domainnetwork.NewSubnetUUID)
	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID,
			CIDR:              "10.0.0.0/24",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Check the subnet entity.
	s.assertSubnet(c, db, subnetUUID, "10.0.0.0/24", 0, spUUID.String())

	// Check the provider network entity.
	s.assertNumProviderNetworkID(c, db, subnetUUID, 0)
	s.assertNumProviderNetworkSubnets(c, db, subnetUUID, 0)

	// Check the provider subnet entity.
	s.assertNumProviderSubnets(c, db, subnetUUID, 0)

	// Check the az entity.
	s.assertSubnetAZs(c, db, subnetUUID, []string{"az0", "az1"})
}

func (s *stateSuite) TestAddSubnetsSameNetworkID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID0 := tc.Must(c, domainnetwork.NewSubnetUUID)
	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID0,
			CIDR:              "10.0.0.0/24",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID1 := tc.Must(c, domainnetwork.NewSubnetUUID)
	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID1,
			CIDR:              "10.0.1.0/24",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	s.assertSubnet(c, db, subnetUUID0, "10.0.0.0/24", 0, spUUID.String())
	s.assertSubnet(c, db, subnetUUID1, "10.0.1.0/24", 0, spUUID.String())
}

func (s *stateSuite) TestFailAddTwoSubnetsSameProviderID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spUUID := networktesting.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID0 := tc.Must(c, domainnetwork.NewSubnetUUID)
	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID0,
			CIDR:              "10.0.0.0/24",
			ProviderId:        "provider-id",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID1 := tc.Must(c, domainnetwork.NewSubnetUUID)
	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID1,
			CIDR:              "10.0.1.0/24",
			ProviderId:        "provider-id",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		}},
	)
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
}

func (s *stateSuite) TestRetrieveFanSubnet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add a subnet of type base.
	subnetUUID0 := tc.Must(c, domainnetwork.NewSubnetUUID)
	err := st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID0,
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0"},
			SpaceID:           "",
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Add a subnet of type fan.
	subnetUUID1 := tc.Must(c, domainnetwork.NewSubnetUUID)
	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID1,
			CIDR:              "10.0.0.0/12",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{"az1"},
			SpaceID:           "",
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	expected := domainnetwork.SubnetInfo{
		UUID:              subnetUUID1,
		CIDR:              "10.0.0.0/12",
		ProviderId:        "provider-id-1",
		ProviderNetworkId: "provider-network-id-1",
		VLANTag:           0,
		AvailabilityZones: []string{"az1"},
		SpaceID:           network.AlphaSpaceId, // alpha space by default.
		SpaceName:         network.AlphaSpaceName,
	}

	// Get the fan subnet by uuid.
	sn1, err := s.getSubnet(c, subnetUUID1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sn1, tc.DeepEquals, expected)

	// Get the fan subnet by cidr.
	subnetsByCIDR, err := st.GetSubnetsByCIDR(c.Context(), "10.0.0.0/12")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnetsByCIDR, tc.HasLen, 1)
	c.Check(subnetsByCIDR[0], tc.DeepEquals, expected)

	// Get all subnets.
	allExpected := domainnetwork.SubnetInfos{
		expected,
		{
			UUID:              subnetUUID0,
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

func (s *stateSuite) TestRetrieveAllSubnets(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add 3 subnets of type base.
	subnetUUID0 := tc.Must(c, domainnetwork.NewSubnetUUID)

	err := st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID0,
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID1 := tc.Must(c, domainnetwork.NewSubnetUUID)

	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID1,
			CIDR:              "192.168.1.0/20",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID2 := tc.Must(c, domainnetwork.NewSubnetUUID)

	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID2,
			CIDR:              "192.168.2.0/20",
			ProviderId:        "provider-id-2",
			ProviderNetworkId: "provider-network-id-2",
			VLANTag:           0,
			AvailabilityZones: []string{"az2", "az3"},
			SpaceID:           "",
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	sns, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sns, tc.HasLen, 3)
}

func (s *stateSuite) TestRetrieveAllSubnet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add 3 subnets of type base.
	subnetUUID0 := tc.Must(c, domainnetwork.NewSubnetUUID)

	err := st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID0,
			CIDR:              "192.168.0.0/20",
			ProviderId:        "0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID1 := tc.Must(c, domainnetwork.NewSubnetUUID)

	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID1,
			CIDR:              "192.168.1.0/20",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID2 := tc.Must(c, domainnetwork.NewSubnetUUID)

	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID2,
			CIDR:              "192.168.2.0/20",
			ProviderId:        "provider-id-2",
			ProviderNetworkId: "provider-network-id-2",
			VLANTag:           0,
			AvailabilityZones: []string{"az2", "az3"},
			SpaceID:           "",
		}},
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

	subnetUUID := tc.Must(c, domainnetwork.NewSubnetUUID)

	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID,
			CIDR:              "10.0.0.0/24",
			ProviderId:        "provider-id",
			ProviderNetworkId: "provider-network-id",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           spUUID,
		}},
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
	subnetUUID0 := tc.Must(c, domainnetwork.NewSubnetUUID)
	err := st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID0,
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
	// Add a subnet of type fan.
	subnetUUID1 := tc.Must(c, domainnetwork.NewSubnetUUID)
	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID1,
			CIDR:              "10.0.0.0/12",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{},
			SpaceID:           "",
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
	// Add another subnet of type fan.
	subnetUUID2 := tc.Must(c, domainnetwork.NewSubnetUUID)
	err = st.ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              subnetUUID2,
			CIDR:              "10.8.0.0/12",
			ProviderId:        "provider-id-2",
			ProviderNetworkId: "provider-network-id-2",
			VLANTag:           0,
			AvailabilityZones: []string{"az4", "az5"},
			SpaceID:           "",
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteSubnet(c.Context(), subnetUUID0.String())
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.getSubnet(c, subnetUUID0)
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
	subnets, err := st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.HasLen, 2)

	err = st.DeleteSubnet(c.Context(), subnetUUID1.String())
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.getSubnet(c, subnetUUID1)
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
	subnets, err = st.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.HasLen, 1)

	err = st.DeleteSubnet(c.Context(), subnetUUID2.String())
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.getSubnet(c, subnetUUID2)
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
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

func (s *stateSuite) assertSubnet(
	c *tc.C,
	db *sql.DB,
	subnetUUID domainnetwork.SubnetUUID,
	expectedCIDR string,
	expectedVLANTag int,
	expectedSpaceUUID string,
) {
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

func (s *stateSuite) getSubnet(c *tc.C, subnetUUID domainnetwork.SubnetUUID) (domainnetwork.SubnetInfo, error) {
	row := s.DB().QueryRowContext(c.Context(), `
SELECT subnet_uuid,
       subnet_cidr,
       COALESCE(subnet_provider_id, ''),
       COALESCE(subnet_provider_space_uuid, ''),
       COALESCE(subnet_provider_network_id, ''),
       subnet_vlan_tag,
       COALESCE(subnet_az, ''),
       subnet_space_uuid,
       subnet_space_name
FROM   v_space_subnet
WHERE  subnet_uuid = ?`, subnetUUID)

	var (
		subnet domainnetwork.SubnetInfo
		az     string
	)
	err := row.Scan(
		&subnet.UUID,
		&subnet.CIDR,
		&subnet.ProviderId,
		&subnet.ProviderSpaceId,
		&subnet.ProviderNetworkId,
		&subnet.VLANTag,
		&az,
		&subnet.SpaceID,
		&subnet.SpaceName,
	)
	if err != nil {
		return domainnetwork.SubnetInfo{}, err
	}
	if az != "" {
		subnet.AvailabilityZones = []string{az}
	}
	return subnet, nil
}

func (s *stateSuite) assertProviderNetworkID(
	c *tc.C,
	db *sql.DB,
	subnetUUID domainnetwork.SubnetUUID,
	expectedProviderNetworkID string,
) {
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

	var retrievedSubnetUUID domainnetwork.SubnetUUID
	err = row.Scan(&retrievedSubnetUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedSubnetUUID, tc.Equals, subnetUUID)
}

func (s *stateSuite) assertNumProviderNetworkID(
	c *tc.C, db *sql.DB, subnetUUID domainnetwork.SubnetUUID, num int,
) {
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

func (s *stateSuite) assertNumProviderNetworkSubnets(
	c *tc.C, db *sql.DB, subnetUUID domainnetwork.SubnetUUID, num int,
) {
	row := db.QueryRow("SELECT COUNT(*) FROM provider_network_subnet WHERE subnet_uuid = ?", subnetUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var numProviderNetworkSubnetID int
	err := row.Scan(&numProviderNetworkSubnetID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(numProviderNetworkSubnetID, tc.Equals, num)
}

func (s *stateSuite) assertProviderSubnetProviderID(
	c *tc.C,
	db *sql.DB,
	subnetUUID domainnetwork.SubnetUUID,
	expectedProviderID string,
) {
	row := db.QueryRow("SELECT provider_id FROM provider_subnet WHERE subnet_uuid = ?", subnetUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var retrievedProviderSubnetID string
	err := row.Scan(&retrievedProviderSubnetID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedProviderSubnetID, tc.Equals, expectedProviderID)
}

func (s *stateSuite) assertNumProviderSubnets(
	c *tc.C, db *sql.DB, subnetUUID domainnetwork.SubnetUUID, num int,
) {
	row := db.QueryRow("SELECT COUNT(*) FROM provider_subnet WHERE subnet_uuid = ?", subnetUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)
	var numProviderSubnetID int
	err := row.Scan(&numProviderSubnetID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(numProviderSubnetID, tc.Equals, num)
}

func (s *stateSuite) assertSubnetAZs(
	c *tc.C,
	db *sql.DB,
	subnetUUID domainnetwork.SubnetUUID,
	expectedAZs []string,
) {
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
