// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"testing"
	"time"

	"github.com/juju/tc"

	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/life"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type relationWithRemoteOfferer struct {
	baseSuite
}

func TestRelationWithRemoteOffererSuite(t *testing.T) {
	tc.Run(t, &relationWithRemoteOfferer{})
}

func (s *relationWithRemoteOfferer) TestRelationWithRemoteOffererExists(c *tc.C) {
	relUUID, _ := s.createRelationWithRemoteOfferer(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationWithRemoteOffererExists(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *relationWithRemoteOfferer) TestRelationWithRemoteOffererExistsFalseForRegularRelation(c *tc.C) {
	relUUID := s.createRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationWithRemoteOffererExists(c.Context(), relUUID.String())

	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *relationWithRemoteOfferer) TestRelationWithRemoteOffererExistsFalseForNonExistingRelation(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationWithRemoteOffererExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *relationWithRemoteOfferer) TestEnsureRelationWithRemoteOffererNotAliveCascadeNormalSuccess(c *tc.C) {
	relUUID, synthAppUUID := s.createRelationWithRemoteOfferer(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid)
SELECT 'local-relation-unit', re.uuid, u.uuid
FROM relation_endpoint AS re
JOIN application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN unit AS u ON ae.application_uuid = u.application_uuid
WHERE re.relation_uuid = ? AND u.name = 'bar/0'`, relUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.EnsureRelationWithRemoteOffererNotAliveCascade(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	var lifeID int
	row := s.DB().QueryRowContext(c.Context(), "SELECT life_id FROM relation where uuid = ?", relUUID.String())
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, int(life.Dying))

	// The synth app should still be alive
	row = s.DB().QueryRowContext(c.Context(), "SELECT life_id FROM application where uuid = ?", synthAppUUID.String())
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, int(life.Alive))

	var units int
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM unit WHERE application_uuid = ?`, synthAppUUID.String()).Scan(&units)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.Equals, 0)
	var settings int
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM relation_unit_setting`).Scan(&settings)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(settings, tc.Equals, 0)
	var settingsHash int
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM relation_unit_settings_hash`).Scan(&settingsHash)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(settingsHash, tc.Equals, 0)
	var archived int
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM relation_unit_setting_archive
WHERE relation_uuid = ? AND key = 'do' AND value = 'da'`, relUUID.String()).Scan(&archived)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(archived, tc.Equals, 3)
	inScope, err := st.UnitNamesInScope(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inScope, tc.DeepEquals, []string{"bar/0"})
}

func (s *relationWithRemoteOfferer) TestEnsureRelationWithRemoteOffererNotAliveCascadeNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	err := st.EnsureRelationWithRemoteOffererNotAliveCascade(c.Context(), "some-relation-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationWithRemoteOfferer) TestEnsureRelationWithRemoteOffererNotAliveCascadeRollback(c *tc.C) {
	relUUID, synthAppUUID := s.createRelationWithRemoteOfferer(c)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	ctx := c.Context()

	_, err := s.DB().ExecContext(ctx, `
CREATE TRIGGER fail_last_synthetic_unit_deletion BEFORE DELETE ON unit
WHEN (SELECT COUNT(*) FROM unit WHERE application_uuid = OLD.application_uuid) = 1
BEGIN
    SELECT RAISE(ABORT, 'synthetic unit deletion failed');
END`)
	c.Assert(err, tc.ErrorIsNil)

	err = st.EnsureRelationWithRemoteOffererNotAliveCascade(ctx, relUUID.String())
	c.Assert(err, tc.ErrorMatches, ".*synthetic unit deletion failed.*")
	relationLife, err := st.GetRelationLife(ctx, relUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relationLife, tc.Equals, life.Alive)

	var aliveUnits int
	err = s.DB().QueryRowContext(ctx, `
SELECT COUNT(*) FROM unit
WHERE application_uuid = ? AND life_id = 0`, synthAppUUID.String()).Scan(&aliveUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(aliveUnits, tc.Equals, 3)
	for _, table := range []string{"relation_unit", "relation_unit_setting", "relation_unit_settings_hash"} {
		var remaining int
		err = s.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&remaining)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(remaining, tc.Equals, 3)
	}
	var archived int
	err = s.DB().QueryRowContext(ctx, `
SELECT COUNT(*) FROM relation_unit_setting_archive WHERE relation_uuid = ?`, relUUID.String()).Scan(&archived)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(archived, tc.Equals, 0)

	_, err = s.DB().ExecContext(ctx, "DROP TRIGGER fail_last_synthetic_unit_deletion")
	c.Assert(err, tc.ErrorIsNil)
	err = st.EnsureRelationWithRemoteOffererNotAliveCascade(ctx, relUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	var totalUnits int
	err = s.DB().QueryRowContext(ctx, `
SELECT COUNT(*) FROM unit WHERE application_uuid = ?`, synthAppUUID.String()).Scan(&totalUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(totalUnits, tc.Equals, 0)
}

func (s *relationWithRemoteOfferer) TestRelationWithRemoteOffererScheduleRemovalNormalSuccess(c *tc.C) {
	relUUID, _ := s.createRelationWithRemoteOfferer(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.RelationWithRemoteOffererScheduleRemoval(
		c.Context(), "removal-uuid", relUUID.String(), false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	row := s.DB().QueryRowContext(c.Context(), `
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

	c.Check(removalType, tc.Equals, "relation with remote offerer")
	c.Check(rUUID, tc.Equals, relUUID.String())
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *relationWithRemoteOfferer) TestRelationWithRemoteOffererScheduleRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.RelationWithRemoteOffererScheduleRemoval(
		c.Context(), "removal-uuid", "some-relation-uuid", true, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	row := s.DB().QueryRowContext(c.Context(), `
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

	c.Check(removalType, tc.Equals, "relation with remote offerer")
	c.Check(rUUID, tc.Equals, "some-relation-uuid")
	c.Check(force, tc.Equals, true)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *relationWithRemoteOfferer) TestDeleteRelationWithRemoteOffererUnitsUnitsStillInScope(c *tc.C) {
	relUUID, _ := s.createRelationWithRemoteOfferer(c)

	s.advanceRelationLife(c, relUUID, life.Dying)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelationWithRemoteOfferer(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIs, removalerrors.UnitsStillInScope)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)
}

func (s *relationWithRemoteOfferer) TestDeleteRelationWithRemoteOffererUnits(c *tc.C) {
	// Arrange
	relUUID, synthAppUUID := s.createRelationWithRemoteOfferer(c)

	s.advanceRelationLife(c, relUUID, life.Dying)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelationUnits(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Act
	err = st.DeleteRelationWithRemoteOfferer(c.Context(), relUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	// The synth app should NOT be deleted.
	row := s.DB().QueryRow("SELECT COUNT(*) FROM application WHERE uuid = ?", synthAppUUID.String())
	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	// But the synth units should be cleaned up.
	row = s.DB().QueryRow("SELECT COUNT(*) FROM unit WHERE application_uuid = ?", synthAppUUID.String())
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *relationWithRemoteOfferer) TestDeleteRelationWithRemoteOffererWhenRemoteAppHasMultipleRelations(c *tc.C) {
	synthAppUUID, _ := s.createRemoteApplicationOfferer(c, "foo")
	s.createIAASApplication(c, s.setupApplicationService(c), "app1",
		applicationservice.AddIAASUnitArg{},
	)
	s.createIAASApplication(c, s.setupApplicationService(c), "app2",
		applicationservice.AddIAASUnitArg{},
	)
	relUUID := s.createRemoteRelationBetween(c, "foo", "app1")
	s.createRemoteRelationBetween(c, "foo", "app2")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelationWithRemoteOfferer(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The synth app should NOT be deleted.
	row := s.DB().QueryRow("SELECT COUNT(*) FROM application WHERE uuid = ?", synthAppUUID.String())
	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	// And the synth units should also NOT be deleted.
	row = s.DB().QueryRow("SELECT COUNT(*) FROM unit WHERE application_uuid = ?", synthAppUUID.String())
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 3)
}
