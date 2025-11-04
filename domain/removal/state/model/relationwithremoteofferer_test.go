// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/life"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type remoteRelationSuite struct {
	baseSuite
}

func TestRelationWithRemoteOffererSuite(t *testing.T) {
	tc.Run(t, &remoteRelationSuite{})
}

func (s *remoteRelationSuite) TestRelationWithRemoteOffererExists(c *tc.C) {
	relUUID, _ := s.createRelationWithRemoteOfferer(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationWithRemoteOffererExists(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *remoteRelationSuite) TestRelationWithRemoteOffererExistsFalseForRegularRelation(c *tc.C) {
	relUUID := s.createRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationWithRemoteOffererExists(c.Context(), relUUID.String())

	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *remoteRelationSuite) TestRelationWithRemoteOffererExistsFalseForNonExistingRelation(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationWithRemoteOffererExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *remoteRelationSuite) TestEnsureRelationWithRemoteOffererNotAliveCascadeNormalSuccess(c *tc.C) {
	relUUID, synthAppUUID := s.createRelationWithRemoteOfferer(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	artifacts, err := st.EnsureRelationWithRemoteOffererNotAliveCascade(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	var lifeID int
	row := s.DB().QueryRowContext(c.Context(), "SELECT life_id FROM relation where uuid = ?", relUUID.String())
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	// The synth app should still be alive
	row = s.DB().QueryRowContext(c.Context(), "SELECT life_id FROM application where uuid = ?", synthAppUUID.String())
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 0)

	// But the synth units should all be dead
	rows, err := s.DB().QueryContext(c.Context(), "SELECT life_id FROM unit where application_uuid = ?", synthAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var lifeID int
		err := rows.Scan(&lifeID)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(lifeID, tc.Equals, 2)
	}

	// Check the returned synth rel units
	synthRelUnitUUIDs := artifacts.SyntheticRelationUnitUUIDs
	c.Check(len(synthRelUnitUUIDs), tc.Equals, 3)

	var gotAppUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT DISTINCT u.application_uuid
FROM            relation_unit AS ru
JOIN            unit AS u ON ru.unit_uuid = u.uuid
WHERE           ru.uuid IN (?, ?, ?)
`, synthRelUnitUUIDs[0], synthRelUnitUUIDs[1], synthRelUnitUUIDs[2]).Scan(&gotAppUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotAppUUID, tc.Equals, synthAppUUID.String())
}

func (s *remoteRelationSuite) TestEnsureRelationWithRemoteOffererNotAliveCascadeNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	_, err := st.EnsureRelationWithRemoteOffererNotAliveCascade(c.Context(), "some-relation-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteRelationSuite) TestRelationWithRemoteOffererScheduleRemovalNormalSuccess(c *tc.C) {
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

func (s *remoteRelationSuite) TestRelationWithRemoteOffererScheduleRemovalNotExistsSuccess(c *tc.C) {
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

func (s *relationSuite) TestDeleteRelationWithRemoteOffererUnitsUnitsStillInScope(c *tc.C) {
	relUUID, _ := s.createRelationWithRemoteOfferer(c)

	s.advanceRelationLife(c, relUUID, life.Dying)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRelationWithRemoteOfferer(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIs, removalerrors.UnitsStillInScope)
}

func (s *relationSuite) TestDeleteRelationWithRemoteOffererUnits(c *tc.C) {
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

func (s *relationSuite) TestDeleteRelationWithRemoteOffererWhenRemoteAppHasMultipleRelations(c *tc.C) {
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
