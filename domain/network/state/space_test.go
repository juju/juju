// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	ctx "context"
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestAddSpace(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Add a subnet of type base.
	subnetUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetUUID.String(),
		"192.168.0.0/12",
		"provider-id-0",
		"provider-network-id-0",
		0,
		[]string{"az0", "az1"},
		"",
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	subnets := []string{subnetUUID.String()}
	err = st.AddSpace(ctx.Background(), uuid, "space0", "foo", subnets)
	c.Assert(err, jc.ErrorIsNil)

	// Check the space entity.
	row := db.QueryRow("SELECT name FROM space WHERE uuid = ?", uuid.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var name string
	err = row.Scan(&name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, "space0")
	// Check the provider id for that space.
	row = db.QueryRow("SELECT provider_id FROM provider_space WHERE space_uuid = ?", uuid.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var providerID string
	err = row.Scan(&providerID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(providerID, gc.Equals, "foo")
	// Check the subnet ids for that space.
	rows, err := db.Query("SELECT uuid FROM subnet WHERE space_uuid = ?", uuid.String())
	c.Assert(err, jc.ErrorIsNil)
	i := 0
	for rows.Next() {
		var subnetID string
		err = rows.Scan(&subnetID)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(subnetID, gc.Equals, subnets[i])
		i++
	}
}

func (s *stateSuite) TestAddSpaceFailDuplicateName(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Add a subnet of type base.
	subnetUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetUUID.String(),
		"192.168.0.0/12",
		"provider-id-0",
		"provider-network-id-0",
		0,
		[]string{"az0", "az1"},
		"",
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	subnets := []string{subnetUUID.String()}
	err = st.AddSpace(ctx.Background(), uuid, "space0", "foo", subnets)
	c.Assert(err, jc.ErrorIsNil)

	// Check the space entity.
	row := db.QueryRow("SELECT name FROM space WHERE uuid = ?", uuid.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var name string
	err = row.Scan(&name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, "space0")
	// Fails when trying to add a new space with the same name.
	err = st.AddSpace(ctx.Background(), uuid, "space0", "bar", subnets)
	c.Assert(err, gc.ErrorMatches, "inserting space (.*) into space table: UNIQUE constraint failed: space.name")

}

func (s *stateSuite) TestAddSpaceEmptyProviderID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Add a subnet of type base.
	subnetUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetUUID.String(),
		"192.168.0.0/12",
		"",
		"provider-network-id-0",
		0,
		[]string{"az0", "az1"},
		"",
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	subnets := []string{subnetUUID.String()}
	err = st.AddSpace(ctx.Background(), uuid, "space0", "", subnets)
	c.Assert(err, jc.ErrorIsNil)

	sp, err := st.GetSpace(ctx.Background(), uuid.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp.ProviderId.String(), gc.Equals, "")

	// Check that no provider space id was added.
	row := db.QueryRow("SELECT provider_id FROM provider_space WHERE space_uuid = ?", uuid.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var spaceProviderID string
	err = row.Scan(&spaceProviderID)
	c.Assert(err, gc.ErrorMatches, "sql: no rows in result set")
}

func (s *stateSuite) TestAddSpaceFailFanOverlay(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Add a subnet of type base.
	subnetUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetUUID.String(),
		"192.168.0.0/12",
		"provider-id-0",
		"provider-network-id-0",
		0,
		[]string{"az0", "az1"},
		"",
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	// Add a subnet of type fan.
	subnetFanUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetFanUUID.String(),
		"10.0.0.0/24",
		"provider-id-1",
		"provider-network-id-1",
		0,
		[]string{"az0", "az1"},
		"",
		&network.FanCIDRs{
			FanLocalUnderlay: "192.168.0.0/12",
			FanOverlay:       "252.0.0.0/8",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	subnets := []string{subnetUUID.String(), subnetFanUUID.String()}
	err = st.AddSpace(ctx.Background(), uuid, "space0", "foo", subnets)

	// Should fail with error indicating we cannot set the space for a
	// FAN subnet.
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("cannot set space for FAN subnet UUIDs \\[%q\\] - it is always inherited from underlay", subnetFanUUID.String()))
}

func (s *stateSuite) TestRetrieveSpaceByUUID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add a subnet of type base.
	subnetUUID0, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetUUID0.String(),
		"192.168.0.0/12",
		"provider-id-0",
		"provider-network-id-0",
		0,
		[]string{"az0"},
		"",
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	// Add a subnet of type base.
	subnetUUID1, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetUUID1.String(),
		"192.176.0.0/12",
		"provider-id-2",
		"provider-network-id-2",
		0,
		[]string{"az1"},
		"",
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	// Add a subnet of type fan.
	subnetFanUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetFanUUID.String(),
		"10.0.0.0/20",
		"provider-id-1",
		"provider-network-id-1",
		0,
		[]string{"az2"},
		"",
		&network.FanCIDRs{
			FanLocalUnderlay: "192.168.0.0/12",
			FanOverlay:       "10.0.0.0/8",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	subnets := []string{subnetUUID0.String(), subnetUUID1.String()}
	spaceUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spaceUUID, "space0", "foo", subnets)
	c.Assert(err, jc.ErrorIsNil)

	sp, err := st.GetSpace(ctx.Background(), spaceUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp.ID, gc.Equals, spaceUUID.String())
	c.Check(sp.Name, gc.Equals, network.SpaceName("space0"))

	expected := network.SubnetInfos{
		{
			ID:                network.Id(subnetUUID0.String()),
			CIDR:              "192.168.0.0/12",
			ProviderId:        "provider-id-0",
			ProviderSpaceId:   "foo",
			ProviderNetworkId: "provider-network-id-0",
			VLANTag:           0,
			AvailabilityZones: []string{"az0"},
			SpaceID:           spaceUUID.String(),
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
			SpaceID:           spaceUUID.String(),
			SpaceName:         "space0",
		},
		{
			ID:                network.Id(subnetFanUUID.String()),
			CIDR:              "10.0.0.0/20",
			ProviderId:        "provider-id-1",
			ProviderSpaceId:   "foo",
			ProviderNetworkId: "provider-network-id-1",
			VLANTag:           0,
			AvailabilityZones: []string{"az2"},
			SpaceID:           spaceUUID.String(),
			SpaceName:         "space0",
		},
	}
	// The 3 subnets must be retrieved (including the overlay segment)
	c.Check(sp.Subnets, gc.HasLen, 3)
	c.Check(sp.Subnets, jc.SameContents, expected)
}

func (s *stateSuite) TestRetrieveSpaceByUUIDNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetSpace(ctx.Background(), "unknown0")
	c.Assert(err, gc.ErrorMatches, "space \"unknown0\" not found")
}

func (s *stateSuite) TestRetrieveSpaceByName(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	spaceUUID0, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spaceUUID0, "space0", "provider0", []string{})
	c.Assert(err, jc.ErrorIsNil)
	spaceUUID1, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spaceUUID1, "space1", "provider1", []string{})
	c.Assert(err, jc.ErrorIsNil)

	sp0, err := st.GetSpaceByName(ctx.Background(), "space0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp0.ID, gc.Equals, spaceUUID0.String())
	c.Check(sp0.Name, gc.Equals, network.SpaceName("space0"))
	sp1, err := st.GetSpaceByName(ctx.Background(), "space1")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp1.ID, gc.Equals, spaceUUID1.String())
	c.Check(sp1.Name, gc.Equals, network.SpaceName("space1"))
}

func (s *stateSuite) TestRetrieveSpaceByNameNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetSpaceByName(ctx.Background(), "unknown0")
	c.Assert(err, gc.ErrorMatches, "space with name \"unknown0\" not found")
}

func (s *stateSuite) TestRetrieveSpaceByUUIDWithoutSubnet(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	spaceUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spaceUUID, "space0", "foo", []string{})
	c.Assert(err, jc.ErrorIsNil)

	sp, err := st.GetSpace(ctx.Background(), spaceUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp.ID, gc.Equals, spaceUUID.String())
	c.Check(sp.Name, gc.Equals, network.SpaceName("space0"))
}

func (s *stateSuite) TestRetrieveAllSpaces(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add 3 subnets of type base.
	subnetUUID0, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetUUID0.String(),
		"192.168.0.0/24",
		"provider-id-0",
		"provider-network-id-0",
		0,
		[]string{"az0", "az1"},
		"",
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	subnetUUID1, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetUUID1.String(),
		"192.168.1.0/24",
		"provider-id-1",
		"provider-network-id-1",
		0,
		[]string{"az0", "az1"},
		"",
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	subnetUUID2, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetUUID2.String(),
		"192.168.2.0/24",
		"provider-id-2",
		"provider-network-id-2",
		0,
		[]string{"az2", "az3"},
		"",
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Create 3 spaces based on the 3 created subnets.
	subnets := []string{subnetUUID0.String()}
	spaceUUID0, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spaceUUID0, "space0", "foo0", subnets)
	c.Assert(err, jc.ErrorIsNil)
	subnets = []string{subnetUUID1.String()}
	spaceUUID1, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spaceUUID1, "space1", "foo1", subnets)
	c.Assert(err, jc.ErrorIsNil)
	subnets = []string{subnetUUID2.String()}
	spaceUUID2, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spaceUUID2, "space2", "foo2", subnets)
	c.Assert(err, jc.ErrorIsNil)

	sp, err := st.GetAllSpaces(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp, gc.HasLen, 4)
}

func (s *stateSuite) TestUpdateSpace(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), uuid, "space0", "foo", []string{})
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateSpace(ctx.Background(), uuid.String(), "newSpaceName0")
	c.Assert(err, jc.ErrorIsNil)

	sp, err := st.GetSpace(ctx.Background(), uuid.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp.Name, gc.Equals, network.SpaceName("newSpaceName0"))
}

func (s *stateSuite) TestUpdateSpaceFailNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.UpdateSpace(ctx.Background(), "unknownSpace", "newSpaceName0")
	c.Assert(err, gc.ErrorMatches, "space \"unknownSpace\" not found")
}

func (s *stateSuite) TestDeleteSpace(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Add a subnet of type base.
	subnetUUID0, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		ctx.Background(),
		subnetUUID0.String(),
		"192.168.0.0/20",
		"provider-id-0",
		"provider-network-id-0",
		0,
		[]string{"az0", "az1"},
		"",
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Create a space containing the newly created subnet.
	spUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(ctx.Background(), spUUID, "space0", "foo", []string{subnetUUID0.String()})
	c.Assert(err, jc.ErrorIsNil)

	// Check the subnet entity.
	row := db.QueryRow("SELECT space_uuid FROM subnet WHERE uuid = ?", subnetUUID0.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var name string
	err = row.Scan(&name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, spUUID.String())

	// Check that the subnet is linked to the newly created space.
	subnet, err := st.GetSubnet(ctx.Background(), subnetUUID0.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnet.SpaceID, gc.Equals, spUUID.String())

	// Delete the space.
	err = st.DeleteSpace(ctx.Background(), spUUID.String())
	c.Assert(err, jc.ErrorIsNil)

	// Check that the subnet is not linked to the deleted space.
	subnet, err = st.GetSubnet(ctx.Background(), subnetUUID0.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnet.SpaceID, gc.Equals, network.AlphaSpaceId)
}
