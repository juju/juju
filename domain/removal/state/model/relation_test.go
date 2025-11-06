// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	crossmodelrelationstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	"github.com/juju/juju/domain/life"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type relationSuite struct {
	baseSuite
}

func TestRelationSuite(t *testing.T) {
	tc.Run(t, &relationSuite{})
}

func (s *relationSuite) TestRelationExists(c *tc.C) {
	relUUID := s.createRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationExists(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *relationSuite) TestRelationExistsDoesNotExist(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *relationSuite) TestRelationExistsCrossModelRelation(c *tc.C) {
	relUUID, _ := s.createRelationWithRemoteOfferer(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationExists(c.Context(), relUUID.String())
	c.Check(exists, tc.Equals, false)
	c.Check(err, tc.ErrorIs, removalerrors.RelationIsCrossModel)
}

func (s *relationSuite) TestEnsureRelationNotAliveNormalSuccess(c *tc.C) {
	relUUID := s.createRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.EnsureRelationNotAlive(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Relation had life "alive" and should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM relation where uuid = ?", relUUID)
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *relationSuite) TestEnsureRelationNotAliveDyingSuccess(c *tc.C) {
	relUUID := s.createRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.EnsureRelationNotAlive(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Relation was already "dying" and should be unchanged.
	row := s.DB().QueryRow("SELECT life_id FROM relation where uuid = ?", relUUID)
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *relationSuite) TestEnsureRelationNotAliveNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	err := st.EnsureRelationNotAlive(c.Context(), "some-relation-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TestRelationRemovalNormalSuccess(c *tc.C) {
	relUUID := s.createRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.RelationScheduleRemoval(
		c.Context(), "removal-uuid", relUUID.String(), false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 0)
	c.Check(rUUID, tc.Equals, relUUID.String())
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *relationSuite) TestRelationRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.RelationScheduleRemoval(
		c.Context(), "removal-uuid", "some-relation-uuid", true, when,
	)
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalType, tc.Equals, "relation")
	c.Check(rUUID, tc.Equals, "some-relation-uuid")
	c.Check(force, tc.Equals, true)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *relationSuite) TestGetRelationLifeSuccess(c *tc.C) {
	relUUID := s.createRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetRelationLife(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Alive)
}

func (s *relationSuite) TestGetRelationLifeDying(c *tc.C) {
	relUUID := s.createRelation(c)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	st.EnsureRelationNotAlive(c.Context(), relUUID.String())

	l, err := st.GetRelationLife(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Dying)
}

func (s *relationSuite) TestGetRelationLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetRelationLife(c.Context(), "some-relation-uuid")
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestUnitNamesInScopeNoRows(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	inScope, err := st.UnitNamesInScope(c.Context(), "some-relation-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inScope, tc.HasLen, 0)
}

func (s *relationSuite) TestUnitNamesInScopeSuccess(c *tc.C) {
	rel, unit, _ := s.addAppUnitRelationScope(c, domaincharm.CharmHubSource)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	inScope, err := st.UnitNamesInScope(c.Context(), rel)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inScope, tc.SameContents, []string{unit})
}

func (s *relationSuite) TestUnitNamesInScopeDropsSyntheticUnits(c *tc.C) {
	rel, _, _ := s.addAppUnitRelationScope(c, domaincharm.CMRSource)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	inScope, err := st.UnitNamesInScope(c.Context(), rel)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inScope, tc.SameContents, []string{})
}

func (s *relationSuite) TestDeleteRelationUnitsSuccess(c *tc.C) {
	rel, _, _ := s.addAppUnitRelationScope(c, domaincharm.CharmHubSource)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelationUnits(c.Context(), rel)
	c.Assert(err, tc.ErrorIsNil)

	inScope, err := st.UnitNamesInScope(c.Context(), rel)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(inScope, tc.HasLen, 0)
}

func (s *relationSuite) TestDeleteRelationUnitsInScopeFails(c *tc.C) {
	rel, _, _ := s.addAppUnitRelationScope(c, domaincharm.CharmHubSource)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelation(c.Context(), rel)
	c.Assert(err, tc.ErrorIs, removalerrors.UnitsStillInScope)
}

func (s *relationSuite) TestDeleteRelationUnitsInScopeSuccess(c *tc.C) {
	rel, _, _ := s.addAppUnitRelationScope(c, domaincharm.CharmHubSource)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelationUnits(c.Context(), rel)
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteRelation(c.Context(), rel)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetRelationLife(c.Context(), rel)
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestDeleteRelationUnitsInScopeSuccessHasSecretPermission(c *tc.C) {
	rel, _, _ := s.addAppUnitRelationScope(c, domaincharm.CharmHubSource)

	_, err := s.DB().Exec(`INSERT INTO secret (id) VALUES (?)`, "secret-id")
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO secret_metadata (secret_id, version, rotate_policy_id)
VALUES (?, ?, ?)`, "secret-id", 1, 0)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO secret_permission (secret_id, role_id, subject_uuid, subject_type_id, scope_uuid, scope_type_id)
VALUES (?, ?, ?, ?, ?, ?)`, "secret-id", 0, "subject-uuid", 0, rel, 3)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteRelationUnits(c.Context(), rel)
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteRelation(c.Context(), rel)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetRelationLife(c.Context(), rel)
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestLeaveScopeSuccess(c *tc.C) {
	// Arrange
	rel, unit, relUnit := s.addAppUnitRelationScope(c, domaincharm.CharmHubSource)

	ctx := c.Context()

	// Add some archived relation settings to simulate this unit
	// leaving and re-entering the relation scope.
	// We expect these to be overwritten.
	s.DB().ExecContext(ctx, `
INSERT INTO relation_unit_setting_archive (relation_uuid, unit_name, "key", value)
VALUES (?, ?, 'old-key', 'old-value')`, rel, unit)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Act
	err := st.LeaveScope(ctx, relUnit)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	inScope, err := st.UnitNamesInScope(c.Context(), rel)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inScope, tc.HasLen, 0)

	// We should have two records in the archive; one each for different units.
	rows, err := s.DB().QueryContext(
		ctx, "SELECT unit_name, key, value FROM relation_unit_setting_archive where relation_uuid = ?", rel)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var (
		unitName, key, val string
		ours, others       int
	)
	for rows.Next() {
		err := rows.Scan(&unitName, &key, &val)
		c.Assert(err, tc.ErrorIsNil)

		if unitName == unit {
			ours++
			c.Check(key, tc.Equals, "key")
			c.Check(val, tc.Equals, "value")
			continue
		}
		others++
	}

	c.Check(ours, tc.Equals, 1)
	c.Check(others, tc.Equals, 1)
}

func (s *relationSuite) TestLeaveScopeDeletesSyntheticUnits(c *tc.C) {
	// Arrange
	s.createRelationWithRemoteOfferer(c)

	var (
		relUnitUUID string
		netNodeUUID string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT ru.uuid
FROM relation_unit AS ru
JOIN unit AS u ON ru.unit_uuid = u.uuid
WHERE u.name = ?`, "foo/0").Scan(&relUnitUUID)
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, `SELECT net_node_uuid FROM unit WHERE name = ?`, "foo/0").Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Act
	err = st.LeaveScope(c.Context(), relUnitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	var (
		unitCount    int
		netNodeCount int
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM unit WHERE name = ?`, "foo/0").Scan(&unitCount)
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM net_node WHERE uuid = ?`, netNodeUUID).Scan(&netNodeCount)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitCount, tc.Equals, 0)
	c.Check(netNodeCount, tc.Equals, 0)
}

func (s *relationSuite) TestLeaveScopeSyntheticUnitsInMultipleRelations(c *tc.C) {
	// Arrange
	synthAppUUID, _ := s.createRemoteApplicationOfferer(c, "foo")

	s.createIAASApplication(c, s.setupApplicationService(c), "bar1",
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{},
	)
	s.createIAASApplication(c, s.setupApplicationService(c), "bar2",
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{},
	)

	relSvc := s.setupRelationService(c)
	_, _, err := relSvc.AddRelation(c.Context(), "foo:foo", "bar1:bar")
	c.Assert(err, tc.ErrorIsNil)
	_, _, err = relSvc.AddRelation(c.Context(), "foo:foo", "bar2:bar")
	c.Assert(err, tc.ErrorIsNil)

	rel1UUID, err := relSvc.GetRelationUUIDForRemoval(c.Context(), domainrelation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:foo", "bar1:bar"},
	})
	c.Assert(err, tc.ErrorIsNil)

	rel2UUID, err := relSvc.GetRelationUUIDForRemoval(c.Context(), domainrelation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:foo", "bar2:bar"},
	})
	c.Assert(err, tc.ErrorIsNil)

	cmrState := crossmodelrelationstate.NewState(
		s.TxnRunnerFactory(), coremodel.UUID(s.ModelUUID()), testclock.NewClock(s.now), loggertesting.WrapCheckLog(c),
	)
	err = cmrState.EnsureUnitsExist(c.Context(), synthAppUUID.String(), []string{"foo/0", "foo/1", "foo/2"})
	c.Assert(err, tc.ErrorIsNil)

	err = relSvc.SetRelationRemoteApplicationAndUnitSettings(c.Context(), synthAppUUID, rel1UUID,
		map[string]string{"do": "da"},
		map[unit.Name]map[string]string{
			unit.Name("foo/0"): {"do": "da"},
			unit.Name("foo/1"): {"do": "da"},
			unit.Name("foo/2"): {"do": "da"},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = relSvc.SetRelationRemoteApplicationAndUnitSettings(c.Context(), synthAppUUID, rel2UUID,
		map[string]string{"da": "do"},
		map[unit.Name]map[string]string{
			unit.Name("foo/0"): {"da": "do"},
			unit.Name("foo/1"): {"da": "do"},
			unit.Name("foo/2"): {"da": "do"},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	var relUnitUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT ru.uuid
FROM relation_unit AS ru
JOIN unit AS u ON ru.unit_uuid = u.uuid
WHERE u.name = ?`, "foo/0").Scan(&relUnitUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Act
	err = st.LeaveScope(c.Context(), relUnitUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TestLeaveScopeRelationUnitNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.LeaveScope(c.Context(), "not-here")
	c.Check(err, tc.ErrorIs, relationerrors.RelationUnitNotFound)
}

// addAppUnitRelationScope adds charm, application, unit and relation
// infrastructure such that a single unit is in the scope of a single relation.
// The relation, unit and relation-unit identifiers are returned.
func (s *relationSuite) addAppUnitRelationScope(c *tc.C, source domaincharm.CharmSource) (string, string, string) {
	charmUUID := "some-charm-uuid"
	// cmr charms don't have an architecture
	if source == domaincharm.CMRSource {
		_, err := s.DB().Exec(
			"INSERT INTO charm (uuid, reference_name, source_id) VALUES (?, ?, ?)",
			charmUUID, charmUUID, encodeCharmSource(c, source))
		c.Assert(err, tc.ErrorIsNil)
	} else {
		_, err := s.DB().Exec(
			"INSERT INTO charm (uuid, reference_name, source_id, architecture_id) VALUES (?, ?, ?, ?)",
			charmUUID, charmUUID, encodeCharmSource(c, source), 0)
		c.Assert(err, tc.ErrorIsNil)
	}

	app := "some-app-uuid"
	_, err := s.DB().Exec(
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)",
		app, app, 0, charmUUID, network.AlphaSpaceId,
	)
	c.Assert(err, tc.ErrorIsNil)

	cr := "some-charm-relation-uuid"
	_, err = s.DB().Exec(`
INSERT INTO charm_relation (uuid, charm_uuid, name, interface, capacity, role_id,  scope_id)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cr, charmUUID, cr, "interface", 0, 0, 0,
	)
	c.Assert(err, tc.ErrorIsNil)

	appEndpoint := "some-app-endpoint-uuid"
	_, err = s.DB().Exec(
		"INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, ?, ?)",
		appEndpoint, app, network.AlphaSpaceId, cr,
	)
	c.Assert(err, tc.ErrorIsNil)

	rel := "some-relation-uuid"
	_, err = s.DB().Exec(
		"INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, ?, ?, ?)", rel, 0, rel, 0)
	c.Assert(err, tc.ErrorIsNil)

	relEndpoint := "some-relation-endpoint-uuid"
	_, err = s.DB().Exec(
		"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)",
		relEndpoint, rel, appEndpoint,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(
		"INSERT INTO relation_application_setting (relation_endpoint_uuid, key, value) VALUES (?, ?, ?)",
		relEndpoint, "key", "value",
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(
		"INSERT INTO relation_application_settings_hash (relation_endpoint_uuid, sha256) VALUES (?, ?)",
		relEndpoint, "hash",
	)
	c.Assert(err, tc.ErrorIsNil)

	node := "some-net-node-uuid"
	_, err = s.DB().Exec("INSERT INTO net_node (uuid) VALUES (?)", node)
	c.Assert(err, tc.ErrorIsNil)

	unit := "some-unit-uuid"
	_, err = s.DB().Exec(
		"INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)",
		unit, unit, 0, app, charmUUID, node)
	c.Assert(err, tc.ErrorIsNil)

	relUnit := "some-rel-unit-uuid"
	_, err = s.DB().Exec("INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid) VALUES (?, ?, ?)",
		relUnit, relEndpoint, unit,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO relation_unit_setting (relation_unit_uuid, key, value) VALUES (?, ?, ?)",
		relUnit, "key", "value",
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO relation_unit_settings_hash (relation_unit_uuid, sha256) VALUES (?, ?)",
		relUnit, "hash",
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(
		"INSERT INTO relation_unit_setting_archive (relation_uuid, unit_name, key, value) VALUES (?, ?, ?, ?)",
		rel, "unit-name-does-not-matter-no-fk", "key", "value",
	)
	c.Assert(err, tc.ErrorIsNil)

	return rel, unit, relUnit
}

func encodeCharmSource(c *tc.C, source domaincharm.CharmSource) int {
	switch source {
	case domaincharm.LocalSource:
		return 0
	case domaincharm.CharmHubSource:
		return 1
	case domaincharm.CMRSource:
		return 2
	default:
		c.Fatalf("unsupported source type: %s", source)
		return -1
	}
}
