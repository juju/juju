// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestAddSpace(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spaceUUID := network.GenSpaceUUID(c)

	// Add a subnet of type base.
	subnetUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID.String()),
			CIDR:              "192.168.0.0/12",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnets := []string{subnetUUID.String()}
	err = st.AddSpace(c.Context(), spaceUUID, "space0", "foo", subnets)
	c.Assert(err, tc.ErrorIsNil)

	// Check the space entity.
	row := db.QueryRow("SELECT name FROM space WHERE uuid = ?", spaceUUID.String())
	c.Assert(row.Err(), tc.ErrorIsNil)
	var name string
	err = row.Scan(&name)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(name, tc.Equals, "space0")
	// Check the provider id for that space.
	row = db.QueryRow("SELECT provider_id FROM provider_space WHERE space_uuid = ?", spaceUUID.String())
	c.Assert(row.Err(), tc.ErrorIsNil)
	var providerID string
	err = row.Scan(&providerID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(providerID, tc.Equals, "foo")
	// Check the subnet ids for that space.
	rows, err := db.Query("SELECT uuid FROM subnet WHERE space_uuid = ?", spaceUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	i := 0
	for rows.Next() {
		var subnetID string
		err = rows.Scan(&subnetID)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(subnetID, tc.Equals, subnets[i])
		i++
	}
}

func (s *stateSuite) TestAddSpaceFailDuplicateName(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spaceUUID := network.GenSpaceUUID(c)

	// Add a subnet of type base.
	subnetUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID.String()),
			CIDR:              "192.168.0.0/12",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnets := []string{subnetUUID.String()}
	err = st.AddSpace(c.Context(), spaceUUID, "space0", "foo", subnets)
	c.Assert(err, tc.ErrorIsNil)

	// Check the space entity.
	row := db.QueryRow("SELECT name FROM space WHERE uuid = ?", spaceUUID.String())
	c.Assert(row.Err(), tc.ErrorIsNil)
	var name string
	err = row.Scan(&name)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(name, tc.Equals, "space0")
	// Fails when trying to add a new space with the same name.
	err = st.AddSpace(c.Context(), spaceUUID, "space0", "bar", subnets)
	c.Assert(err, tc.ErrorIs, networkerrors.SpaceAlreadyExists)
}

func (s *stateSuite) TestAddSpaceEmptyProviderID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spaceUUID := network.GenSpaceUUID(c)

	// Add a subnet of type base.
	subnetUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID.String()),
			CIDR:              "192.168.0.0/12",
			ProviderId:        "",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnets := []string{subnetUUID.String()}
	err = st.AddSpace(c.Context(), spaceUUID, "space0", "", subnets)
	c.Assert(err, tc.ErrorIsNil)

	sp, err := st.GetSpace(c.Context(), spaceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sp.ProviderId.String(), tc.Equals, "")

	// Check that no provider space id was added.
	row := db.QueryRow("SELECT provider_id FROM provider_space WHERE space_uuid = ?", spaceUUID.String())
	c.Assert(row.Err(), tc.ErrorIsNil)
	var spaceProviderID string
	err = row.Scan(&spaceProviderID)
	c.Assert(err, tc.ErrorMatches, "sql: no rows in result set")
}

func (s *stateSuite) TestRetrieveSpaceByUUID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add a subnet of type base.
	subnetUUID0, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "192.168.0.0/12",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	// Add a subnet of type base.
	subnetUUID1, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID1.String()),
			CIDR:              "192.176.0.0/12",
			ProviderId:        "provider-id-2",
			ProviderNetworkId: "provider-network-id-2",
			VLANTag:           0,
			AvailabilityZones: []string{"az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	subnets := []string{subnetUUID0.String(), subnetUUID1.String()}
	spaceUUID := network.GenSpaceUUID(c)
	err = st.AddSpace(c.Context(), spaceUUID, "space0", "foo", subnets)
	c.Assert(err, tc.ErrorIsNil)

	sp, err := st.GetSpace(c.Context(), spaceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sp.ID, tc.Equals, spaceUUID)
	c.Check(sp.Name, tc.Equals, network.SpaceName("space0"))

	expected := network.SubnetInfos{
		{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "192.168.0.0/12",
			ProviderId:        "provider-id-0",
			ProviderSpaceId:   "foo",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0"},
			SpaceID:           spaceUUID,
			SpaceName:         "space0",
		},
		{
			ID:                network.Id(subnetUUID1.String()),
			CIDR:              "192.176.0.0/12",
			ProviderId:        "provider-id-2",
			ProviderSpaceId:   "foo",
			ProviderNetworkId: "provider-network-id-2",
			VLANTag:           0,
			AvailabilityZones: []string{"az1"},
			SpaceID:           spaceUUID,
			SpaceName:         "space0",
		},
	}
	// The 3 subnets must be retrieved (including the overlay segment)
	c.Check(sp.Subnets, tc.HasLen, 2)
	c.Check(sp.Subnets, tc.SameContents, expected)
}

// TestRetrieveSpaceByUUIDNotFound tests that if we try to call State.GetSpace
// with a non-existent space, it will return an error matching
// [networkerrors.SpaceNotFound].
func (s *stateSuite) TestRetrieveSpaceByUUIDNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetSpace(c.Context(), "unknown0")
	c.Assert(err, tc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *stateSuite) TestRetrieveSpaceByName(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spaceUUID0 := network.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spaceUUID0, "space0", "provider0", []string{})
	c.Assert(err, tc.ErrorIsNil)
	spaceUUID1 := network.GenSpaceUUID(c)
	err = st.AddSpace(c.Context(), spaceUUID1, "space1", "provider1", []string{})
	c.Assert(err, tc.ErrorIsNil)

	sp0, err := st.GetSpaceByName(c.Context(), "space0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sp0.ID, tc.Equals, spaceUUID0)
	c.Check(sp0.Name, tc.Equals, network.SpaceName("space0"))
	sp1, err := st.GetSpaceByName(c.Context(), "space1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sp1.ID, tc.Equals, spaceUUID1)
	c.Check(sp1.Name, tc.Equals, network.SpaceName("space1"))
}

// TestRetrieveSpaceByNameNotFound tests that if we try to call
// State.GetSpaceByName with a non-existent space, it will return an error
// matching [networkerrors.SpaceNotFound].
func (s *stateSuite) TestRetrieveSpaceByNameNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetSpaceByName(c.Context(), "unknown0")
	c.Assert(err, tc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *stateSuite) TestRetrieveSpaceByUUIDWithoutSubnet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spaceUUID := network.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), spaceUUID, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	sp, err := st.GetSpace(c.Context(), spaceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sp.ID, tc.Equals, spaceUUID)
	c.Check(sp.Name, tc.Equals, network.SpaceName("space0"))
	c.Check(sp.Subnets, tc.IsNil)
}

func (s *stateSuite) TestRetrieveAllSpaces(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add 3 subnets of type base.
	subnetUUID0, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "192.168.0.0/24",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	subnetUUID1, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID1.String()),
			CIDR:              "192.168.1.0/24",
			ProviderId:        "provider-id-1",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{"az0", "az1"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	subnetUUID2, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddSubnet(
		c.Context(),
		network.SubnetInfo{
			ID:                network.Id(subnetUUID2.String()),
			CIDR:              "192.168.2.0/24",
			ProviderId:        "provider-id-2",
			ProviderNetworkId: "provider-network-id-2",
			VLANTag:           0,
			AvailabilityZones: []string{"az2", "az3"},
			SpaceID:           "",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Create 3 spaces based on the 3 created subnets.
	subnets := []string{subnetUUID0.String()}
	spaceUUID0 := network.GenSpaceUUID(c)
	err = st.AddSpace(c.Context(), spaceUUID0, "space0", "foo0", subnets)
	c.Assert(err, tc.ErrorIsNil)

	subnets = []string{subnetUUID1.String()}
	spaceUUID1 := network.GenSpaceUUID(c)
	err = st.AddSpace(c.Context(), spaceUUID1, "space1", "foo1", subnets)
	c.Assert(err, tc.ErrorIsNil)

	subnets = []string{subnetUUID2.String()}
	spaceUUID2 := network.GenSpaceUUID(c)
	err = st.AddSpace(c.Context(), spaceUUID2, "space2", "foo2", subnets)
	c.Assert(err, tc.ErrorIsNil)

	sp, err := st.GetAllSpaces(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sp, tc.HasLen, 4)
}

func (s *stateSuite) TestUpdateSpace(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	uuid := network.GenSpaceUUID(c)
	err := st.AddSpace(c.Context(), uuid, "space0", "foo", []string{})
	c.Assert(err, tc.ErrorIsNil)

	err = st.UpdateSpace(c.Context(), uuid, "newSpaceName0")
	c.Assert(err, tc.ErrorIsNil)

	sp, err := st.GetSpace(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sp.Name, tc.Equals, network.SpaceName("newSpaceName0"))
}

// TestUpdateSpaceFailNotFound tests that if we try to call State.UpdateSpace
// with a non-existent space, it will return an error matching
// [networkerrors.SpaceNotFound].
func (s *stateSuite) TestUpdateSpaceFailNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.UpdateSpace(c.Context(), "unknownSpace", "newSpaceName0")
	c.Assert(err, tc.ErrorIs, networkerrors.SpaceNotFound)
}
