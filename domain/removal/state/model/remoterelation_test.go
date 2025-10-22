// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"testing"
	"time"

	"github.com/juju/tc"

	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/life"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type remoteRelationSuite struct {
	baseSuite
}

func TestRemoteRelationSuite(t *testing.T) {
	tc.Run(t, &remoteRelationSuite{})
}

func (s *remoteRelationSuite) TestRemoteRelationExists(c *tc.C) {
	relUUID, _ := s.createRemoteRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RemoteRelationExists(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *remoteRelationSuite) TestRemoteRelationExistsFalseForRegularRelation(c *tc.C) {
	relUUID := s.createRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RemoteRelationExists(c.Context(), relUUID.String())

	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *remoteRelationSuite) TestRemoteRelationExistsFalseForNonExistingRelation(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RemoteRelationExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *remoteRelationSuite) TestEnsureRemoteRelationNotAliveCascadeNormalSuccess(c *tc.C) {
	relUUID, synthAppUUID := s.createRemoteRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.EnsureRemoteRelationNotAliveCascade(c.Context(), relUUID.String())
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
}

func (s *remoteRelationSuite) TestEnsureRemoteRelationNotAliveCascadeNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	err := st.EnsureRemoteRelationNotAliveCascade(c.Context(), "some-relation-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteRelationSuite) TestRemoteRelationScheduleRemovalNormalSuccess(c *tc.C) {
	relUUID, _ := s.createRemoteRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.RemoteRelationScheduleRemoval(
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

	c.Check(removalType, tc.Equals, "remote relation")
	c.Check(rUUID, tc.Equals, relUUID.String())
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *remoteRelationSuite) TestRemoteRelationScheduleRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.RemoteRelationScheduleRemoval(
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

	c.Check(removalType, tc.Equals, "remote relation")
	c.Check(rUUID, tc.Equals, "some-relation-uuid")
	c.Check(force, tc.Equals, true)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *relationSuite) TestDeleteRemoteRelation(c *tc.C) {
	relUUID, synthAppUUID := s.createRemoteRelation(c)

	s.advanceRelationLife(c, relUUID, life.Dying)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRemoteRelation(c.Context(), relUUID.String())
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

func (s *relationSuite) TestDeleteRemoteRelationWhenRemoteAppHasMultipleRelations(c *tc.C) {
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

	err := st.DeleteRemoteRelation(c.Context(), relUUID.String())
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
