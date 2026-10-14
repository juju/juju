// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"testing"
	"time"

	"github.com/juju/tc"

	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/life"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type relationWithRemoteConsumer struct {
	baseSuite
}

func TestRelationWithRemoteConsumerSuite(t *testing.T) {
	tc.Run(t, &relationWithRemoteConsumer{})
}

func (s *relationWithRemoteConsumer) TestRelationWithRemoteConsumerExists(c *tc.C) {
	relUUID, _, _ := s.createRelationWithRemoteConsumer(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationWithRemoteConsumerExists(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *relationWithRemoteConsumer) TestRelationWithRemoteConsumerExistsFalseForRegularRelation(c *tc.C) {
	relUUID := s.createRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationWithRemoteConsumerExists(c.Context(), relUUID.String())

	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *relationWithRemoteConsumer) TestRelationWithRemoteConsumerExistsFalseForNonExistingRelation(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationWithRemoteConsumerExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *relationWithRemoteConsumer) TestEnsureRelationWithRemoteConsumerNotAliveCascadeNormalSuccess(c *tc.C) {
	relUUID, synthAppUUID, _ := s.createRelationWithRemoteConsumer(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.EnsureRelationWithRemoteConsumerNotAliveCascade(c.Context(), relUUID.String())
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

	// The synthetic units are departed and deleted in the same transaction,
	// and their settings are archived.
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
}

// TestEnsureRelationWithRemoteConsumerNotAliveCascadeSharedSyntheticUnits
// checks that removing one relation departs only its own synthetic relation
// units, retaining units that remain in scope of another relation.
func (s *relationWithRemoteConsumer) TestEnsureRelationWithRemoteConsumerNotAliveCascadeSharedSyntheticUnits(c *tc.C) {
	rel1UUID, synthAppUUID, _ := s.createRelationWithRemoteConsumer(c)
	rel2UUID := s.addRelationSharingSyntheticApp(c, rel1UUID.String(), synthAppUUID.String())

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.EnsureRelationWithRemoteConsumerNotAliveCascade(c.Context(), rel1UUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The first relation's synthetic relation units are gone, the second
	// relation is untouched.
	var inScope int
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM relation_unit AS ru
JOIN relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
WHERE re.relation_uuid = ?`, rel1UUID.String()).Scan(&inScope)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inScope, tc.Equals, 0)
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM relation_unit AS ru
JOIN relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
WHERE re.relation_uuid = ?`, rel2UUID).Scan(&inScope)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inScope, tc.Equals, 3)

	// The shared synthetic units are retained while the second relation is
	// alive.
	var aliveUnits int
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM unit WHERE application_uuid = ? AND life_id = 0`, synthAppUUID.String()).Scan(&aliveUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(aliveUnits, tc.Equals, 3)
	relationLife, err := st.GetRelationLife(c.Context(), rel2UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relationLife, tc.Equals, life.Alive)

	// Removing the second relation departs the remaining relation units and
	// deletes the synthetic units.
	err = st.EnsureRelationWithRemoteConsumerNotAliveCascade(c.Context(), rel2UUID)
	c.Assert(err, tc.ErrorIsNil)
	var units int
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM unit WHERE application_uuid = ?`, synthAppUUID.String()).Scan(&units)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.Equals, 0)
}

// addRelationSharingSyntheticApp adds a second relation over the synthetic
// consumer application's endpoint, placing the same synthetic units in scope
// of both relations.
func (s *relationWithRemoteConsumer) addRelationSharingSyntheticApp(c *tc.C, relUUID, synthAppUUID string) string {
	rel2UUID := tc.Must(c, corerelation.NewUUID)
	rel2EndpointUUID := tc.Must(c, corerelation.NewUUID)
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
SELECT ?, life_id, (SELECT MAX(relation_id) + 1 FROM relation), scope_id
FROM   relation
WHERE  uuid = ?`, rel2UUID.String(), relUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
SELECT ?, ?, re.endpoint_uuid
FROM   relation_endpoint AS re
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN   application AS a ON ae.application_uuid = a.uuid
WHERE  re.relation_uuid = ? AND a.uuid = ?`, rel2EndpointUUID.String(), rel2UUID.String(), relUUID, synthAppUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid)
SELECT ru.uuid || '-rel2', ?, ru.unit_uuid
FROM   relation_unit AS ru
JOIN   relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
WHERE  re.relation_uuid = ?`, rel2EndpointUUID.String(), relUUID)
	c.Assert(err, tc.ErrorIsNil)
	return rel2UUID.String()
}

func (s *relationWithRemoteConsumer) TestEnsureRelationWithRemoteConsumerNotAliveCascadeNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	err := st.EnsureRelationWithRemoteConsumerNotAliveCascade(c.Context(), "some-relation-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationWithRemoteConsumer) TestRelationWithRemoteConsumerScheduleRemovalNormalSuccess(c *tc.C) {
	relUUID, _, _ := s.createRelationWithRemoteConsumer(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.RelationWithRemoteConsumerScheduleRemoval(
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

	c.Check(removalType, tc.Equals, "relation with remote consumer")
	c.Check(rUUID, tc.Equals, relUUID.String())
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *relationWithRemoteConsumer) TestRelationWithRemoteConsumerScheduleRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.RelationWithRemoteConsumerScheduleRemoval(
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

	c.Check(removalType, tc.Equals, "relation with remote consumer")
	c.Check(rUUID, tc.Equals, "some-relation-uuid")
	c.Check(force, tc.Equals, true)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *relationWithRemoteConsumer) TestDeleteRelationWithRemoteConsumerUnitsUnitsStillInScope(c *tc.C) {
	relUUID, _, _ := s.createRelationWithRemoteConsumer(c)

	s.advanceRelationLife(c, relUUID, life.Dying)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelationWithRemoteConsumer(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIs, removalerrors.UnitsStillInScope)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)
}

func (s *relationWithRemoteConsumer) TestDeleteRelationWithRemoteConsumerUnits(c *tc.C) {
	// Arrange
	relUUID, synthAppUUID, _ := s.createRelationWithRemoteConsumer(c)

	s.advanceRelationLife(c, relUUID, life.Dying)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelationUnits(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Act
	err = st.DeleteRelationWithRemoteConsumer(c.Context(), relUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	// The synth app should be deleted.
	row := s.DB().QueryRow("SELECT COUNT(*) FROM application WHERE uuid = ?", synthAppUUID.String())
	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}
