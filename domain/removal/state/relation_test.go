// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type relationSuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&relationSuite{})

func (s *relationSuite) TestRelationExists(c *tc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 0, "some-relation-id")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationExists(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)

	exists, err = st.RelationExists(context.Background(), "not-today-henry")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *relationSuite) TestEnsureRelationNotAliveNormalSuccess(c *tc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 0, "some-relation-id")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.EnsureRelationNotAlive(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIsNil)

	// Relation had life "alive" and should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM relation where uuid = ?", "some-relation-uuid")
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *relationSuite) TestEnsureRelationNotAliveDyingSuccess(c *tc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 1, "some-relation-id")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.EnsureRelationNotAlive(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIsNil)

	// Relation was already "dying" and should be unchanged.
	row := s.DB().QueryRow("SELECT life_id FROM relation where uuid = ?", "some-relation-uuid")
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *relationSuite) TestEnsureRelationNotAliveNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	err := st.EnsureRelationNotAlive(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationSuite) TestRelationRemovalNormalSuccess(c *tc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 1, "some-relation-id")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err = st.RelationScheduleRemoval(
		context.Background(), "removal-uuid", "some-relation-uuid", false, when,
	)
	c.Assert(err, jc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 0)
	c.Check(rUUID, tc.Equals, "some-relation-uuid")
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *relationSuite) TestRelationRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.RelationScheduleRemoval(
		context.Background(), "removal-uuid", "some-relation-uuid", true, when,
	)
	c.Assert(err, jc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	// It doesn't matter that the relation does not exist.
	// We rely on the worker to handle that fact.
	row := s.DB().QueryRow(`
SELECT t.name, r.entity_uuid, r.force, r.scheduled_for 
FROM   removal r JOIN removal_type t ON r.removal_type_id = t.id
where  r.uuid = ?`, "removal-uuid",
	)

	var (
		removalType  string
		rUUID        string
		force        bool
		scheduledFor time.Time
	)
	err = row.Scan(&removalType, &rUUID, &force, &scheduledFor)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(removalType, tc.Equals, "relation")
	c.Check(rUUID, tc.Equals, "some-relation-uuid")
	c.Check(force, tc.Equals, true)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *relationSuite) TestGetRelationLifeSuccess(c *tc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 1, "some-relation-id")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetRelationLife(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Dying)
}

func (s *relationSuite) TestGetRelationLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetRelationLife(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestUnitNamesInScopeNoRows(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	inScope, err := st.UnitNamesInScope(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(inScope, tc.HasLen, 0)
}

func (s *relationSuite) TestUnitNamesInScopeSuccess(c *tc.C) {
	rel, unit := s.addAppUnitRelationScope(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	inScope, err := st.UnitNamesInScope(context.Background(), rel)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(inScope, jc.SameContents, []string{unit})
}

func (s *relationSuite) TestDeleteRelationUnitsSuccess(c *tc.C) {
	rel, _ := s.addAppUnitRelationScope(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelationUnits(context.Background(), rel)
	c.Assert(err, jc.ErrorIsNil)

	inScope, err := st.UnitNamesInScope(context.Background(), rel)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(inScope, tc.HasLen, 0)
}

func (s *relationSuite) TestDeleteRelationUnitsInScopeFails(c *tc.C) {
	rel, _ := s.addAppUnitRelationScope(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelation(context.Background(), rel)
	c.Assert(err, jc.ErrorIs, errors.UnitsStillInScope)
}

func (s *relationSuite) TestDeleteRelationUnitsInScopeSuccess(c *tc.C) {
	rel, _ := s.addAppUnitRelationScope(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelationUnits(context.Background(), rel)
	c.Assert(err, jc.ErrorIsNil)

	err = st.DeleteRelation(context.Background(), rel)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.GetRelationLife(context.Background(), rel)
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

// addAppUnitRelationScope adds charm, application, unit and relation
// infrastructure such that a single unit is in the scope of a single relation.
// The relation and unit identifiers are returned.
func (s *relationSuite) addAppUnitRelationScope(c *tc.C) (string, string) {
	charm := "some-charm-uuid"
	_, err := s.DB().Exec("INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, ?)", charm, charm, 0)
	c.Assert(err, jc.ErrorIsNil)

	app := "some-app-uuid"
	_, err = s.DB().Exec(
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)",
		app, app, 0, charm, network.AlphaSpaceId,
	)
	c.Assert(err, jc.ErrorIsNil)

	cr := "some-charm-relation-uuid"
	_, err = s.DB().Exec(`
INSERT INTO charm_relation (uuid, charm_uuid, name, interface, capacity, role_id,  scope_id)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cr, charm, cr, "interface", 0, 0, 0,
	)
	c.Assert(err, jc.ErrorIsNil)

	appEndpoint := "some-app-endpoint-uuid"
	_, err = s.DB().Exec(
		"INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, ?, ?)",
		appEndpoint, app, network.AlphaSpaceId, cr,
	)
	c.Assert(err, jc.ErrorIsNil)

	rel := "some-relation-uuid"
	_, err = s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)", rel, 0, rel)
	c.Assert(err, jc.ErrorIsNil)

	relEndpoint := "some-relation-endpoint-uuid"
	_, err = s.DB().Exec(
		"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)",
		relEndpoint, rel, appEndpoint,
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO relation_application_setting (relation_endpoint_uuid, key, value) VALUES (?, ?, ?)",
		relEndpoint, "key", "value",
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO relation_application_settings_hash (relation_endpoint_uuid, sha256) VALUES (?, ?)",
		relEndpoint, "hash",
	)
	c.Assert(err, jc.ErrorIsNil)

	node := "some-net-node-uuid"
	_, err = s.DB().Exec("INSERT INTO net_node (uuid) VALUES (?)", node)
	c.Assert(err, jc.ErrorIsNil)

	unit := "some-unit-uuid"
	_, err = s.DB().Exec(
		"INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)",
		unit, unit, 0, app, charm, node)
	c.Assert(err, jc.ErrorIsNil)

	relUnit := "some-rel-unit-uuid"
	_, err = s.DB().Exec("INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid) VALUES (?, ?, ?)",
		relUnit, relEndpoint, unit,
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO relation_unit_setting (relation_unit_uuid, key, value) VALUES (?, ?, ?)",
		"some-rel-unit-uuid", "key", "value",
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO relation_unit_settings_hash (relation_unit_uuid, sha256) VALUES (?, ?)",
		"some-rel-unit-uuid", "hash",
	)
	c.Assert(err, jc.ErrorIsNil)

	return rel, unit
}
