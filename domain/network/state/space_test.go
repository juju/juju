// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestAddSpace(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spaceUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Add a subnet of type base.
	subnetUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)

	subnets := []string{subnetUUID.String()}
	err = st.AddSpace(context.Background(), spaceUUID.String(), "space0", "foo", subnets)
	c.Assert(err, jc.ErrorIsNil)

	// Check the space entity.
	row := db.QueryRow("SELECT name FROM space WHERE uuid = ?", spaceUUID.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var name string
	err = row.Scan(&name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, "space0")
	// Check the provider id for that space.
	row = db.QueryRow("SELECT provider_id FROM provider_space WHERE space_uuid = ?", spaceUUID.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var providerID string
	err = row.Scan(&providerID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(providerID, gc.Equals, "foo")
	// Check the subnet ids for that space.
	rows, err := db.Query("SELECT uuid FROM subnet WHERE space_uuid = ?", spaceUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

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
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spaceUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Add a subnet of type base.
	subnetUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)

	subnets := []string{subnetUUID.String()}
	err = st.AddSpace(context.Background(), spaceUUID.String(), "space0", "foo", subnets)
	c.Assert(err, jc.ErrorIsNil)

	// Check the space entity.
	row := db.QueryRow("SELECT name FROM space WHERE uuid = ?", spaceUUID.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var name string
	err = row.Scan(&name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, "space0")
	// Fails when trying to add a new space with the same name.
	err = st.AddSpace(context.Background(), spaceUUID.String(), "space0", "bar", subnets)
	c.Assert(err, jc.ErrorIs, networkerrors.SpaceAlreadyExists)
}

func (s *stateSuite) TestAddSpaceEmptyProviderID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	spaceUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Add a subnet of type base.
	subnetUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)

	subnets := []string{subnetUUID.String()}
	err = st.AddSpace(context.Background(), spaceUUID.String(), "space0", "", subnets)
	c.Assert(err, jc.ErrorIsNil)

	sp, err := st.GetSpace(context.Background(), spaceUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp.ProviderId.String(), gc.Equals, "")

	// Check that no provider space id was added.
	row := db.QueryRow("SELECT provider_id FROM provider_space WHERE space_uuid = ?", spaceUUID.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var spaceProviderID string
	err = row.Scan(&spaceProviderID)
	c.Assert(err, gc.ErrorMatches, "sql: no rows in result set")
}

func (s *stateSuite) TestRetrieveSpaceByUUID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add a subnet of type base.
	subnetUUID0, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)
	// Add a subnet of type base.
	subnetUUID1, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)

	subnets := []string{subnetUUID0.String(), subnetUUID1.String()}
	spaceUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(context.Background(), spaceUUID.String(), "space0", "foo", subnets)
	c.Assert(err, jc.ErrorIsNil)

	sp, err := st.GetSpace(context.Background(), spaceUUID.String())
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
	}
	// The 3 subnets must be retrieved (including the overlay segment)
	c.Check(sp.Subnets, gc.HasLen, 2)
	c.Check(sp.Subnets, jc.SameContents, expected)
}

// TestRetrieveSpaceByUUIDNotFound tests that if we try to call State.GetSpace
// with a non-existent space, it will return an error matching
// [networkerrors.SpaceNotFound].
func (s *stateSuite) TestRetrieveSpaceByUUIDNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetSpace(context.Background(), "unknown0")
	c.Assert(err, jc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *stateSuite) TestRetrieveSpaceByName(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spaceUUID0, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(context.Background(), spaceUUID0.String(), "space0", "provider0", []string{})
	c.Assert(err, jc.ErrorIsNil)
	spaceUUID1, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(context.Background(), spaceUUID1.String(), "space1", "provider1", []string{})
	c.Assert(err, jc.ErrorIsNil)

	sp0, err := st.GetSpaceByName(context.Background(), "space0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp0.ID, gc.Equals, spaceUUID0.String())
	c.Check(sp0.Name, gc.Equals, network.SpaceName("space0"))
	sp1, err := st.GetSpaceByName(context.Background(), "space1")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp1.ID, gc.Equals, spaceUUID1.String())
	c.Check(sp1.Name, gc.Equals, network.SpaceName("space1"))
}

// TestRetrieveSpaceByNameNotFound tests that if we try to call
// State.GetSpaceByName with a non-existent space, it will return an error
// matching [networkerrors.SpaceNotFound].
func (s *stateSuite) TestRetrieveSpaceByNameNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetSpaceByName(context.Background(), "unknown0")
	c.Assert(err, jc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *stateSuite) TestRetrieveSpaceByUUIDWithoutSubnet(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spaceUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(context.Background(), spaceUUID.String(), "space0", "foo", []string{})
	c.Assert(err, jc.ErrorIsNil)

	sp, err := st.GetSpace(context.Background(), spaceUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp.ID, gc.Equals, spaceUUID.String())
	c.Check(sp.Name, gc.Equals, network.SpaceName("space0"))
	c.Check(sp.Subnets, gc.IsNil)
}

func (s *stateSuite) TestRetrieveAllSpaces(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Add 3 subnets of type base.
	subnetUUID0, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)
	subnetUUID1, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)
	subnetUUID2, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)

	// Create 3 spaces based on the 3 created subnets.
	subnets := []string{subnetUUID0.String()}
	spaceUUID0, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(context.Background(), spaceUUID0.String(), "space0", "foo0", subnets)
	c.Assert(err, jc.ErrorIsNil)
	subnets = []string{subnetUUID1.String()}
	spaceUUID1, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(context.Background(), spaceUUID1.String(), "space1", "foo1", subnets)
	c.Assert(err, jc.ErrorIsNil)
	subnets = []string{subnetUUID2.String()}
	spaceUUID2, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(context.Background(), spaceUUID2.String(), "space2", "foo2", subnets)
	c.Assert(err, jc.ErrorIsNil)

	sp, err := st.GetAllSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp, gc.HasLen, 4)
}

func (s *stateSuite) TestUpdateSpace(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(context.Background(), uuid.String(), "space0", "foo", []string{})
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateSpace(context.Background(), uuid.String(), "newSpaceName0")
	c.Assert(err, jc.ErrorIsNil)

	sp, err := st.GetSpace(context.Background(), uuid.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sp.Name, gc.Equals, network.SpaceName("newSpaceName0"))
}

// TestUpdateSpaceFailNotFound tests that if we try to call State.UpdateSpace
// with a non-existent space, it will return an error matching
// [networkerrors.SpaceNotFound].
func (s *stateSuite) TestUpdateSpaceFailNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.UpdateSpace(context.Background(), "unknownSpace", "newSpaceName0")
	c.Assert(err, jc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *stateSuite) TestDeleteSpace(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	// Add a subnet of type base.
	subnetUUID0, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSubnet(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)

	// Create a space containing the newly created subnet.
	spUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(context.Background(), spUUID.String(), "space0", "foo", []string{subnetUUID0.String()})
	c.Assert(err, jc.ErrorIsNil)

	// Check the subnet entity.
	row := db.QueryRow("SELECT space_uuid FROM subnet WHERE uuid = ?", subnetUUID0.String())
	c.Assert(row.Err(), jc.ErrorIsNil)
	var name string
	err = row.Scan(&name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, spUUID.String())

	// Check that the subnet is linked to the newly created space.
	subnet, err := st.GetSubnet(context.Background(), subnetUUID0.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnet.SpaceID, gc.Equals, spUUID.String())

	// Delete the space.
	err = st.DeleteSpace(context.Background(), spUUID.String())
	c.Assert(err, jc.ErrorIsNil)

	// Check that the subnet is not linked to the deleted space.
	subnet, err = st.GetSubnet(context.Background(), subnetUUID0.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnet.SpaceID, gc.Equals, network.AlphaSpaceId)
}

// TestDeleteSpaceNotFound tests that if we try to call State.DeleteSpace with
// a non-existent space, it will return an error matching
// [networkerrors.SpaceNotFound].
func (s *stateSuite) TestDeleteSpaceNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	spUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.DeleteSpace(context.Background(), spUUID.String())
	c.Assert(err, jc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *stateSuite) TestIsSpaceNotUsedInConstraints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Create a space.
	spUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(context.Background(), spUUID.String(), "space0", "foo", []string{})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the space is used in constraints.
	used, err := st.IsSpaceUsedInConstraints(context.Background(), "space0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(used, jc.IsFalse)
}

func (s *stateSuite) TestIsSpaceUsedInApplicationConstraints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Create a space.
	spUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddSpace(context.Background(), spUUID.String(), "space0", "foo", []string{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		addConstraintStmt := `INSERT INTO "constraint" (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, addConstraintStmt, "constraint-uuid")
		if err != nil {
			return err
		}
		addSpaceConsStmt := `INSERT INTO constraint_space (constraint_uuid, space, exclude) VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, addSpaceConsStmt, "constraint-uuid", "space0", false)
		if err != nil {
			return err
		}
		addCharmStmt := `INSERT INTO charm (uuid, reference_name, source_id) VALUES (?, 'foo', 0)`
		_, err = tx.ExecContext(ctx, addCharmStmt, "charm0-uuid")
		if err != nil {
			return errors.Capture(err)
		}
		addApplicationStmt := `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, addApplicationStmt, "app0-uuid", "app0", "0", "charm0-uuid", network.AlphaSpaceId)
		if err != nil {
			return err
		}
		addAppConstraintStmt := `INSERT INTO application_constraint (application_uuid, constraint_uuid) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addAppConstraintStmt, "app0-uuid", "constraint-uuid")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the space is used in constraints.
	used, err := st.IsSpaceUsedInConstraints(context.Background(), "space0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(used, jc.IsTrue)

	// Check that the space is not used in constraints.
	used, err = st.IsSpaceUsedInConstraints(context.Background(), "space1")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(used, jc.IsFalse)
}
