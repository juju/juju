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
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type charmSuite struct {
	baseSuite
}

func TestCharmSuite(t *testing.T) {
	tc.Run(t, &charmSuite{})
}

func (s *charmSuite) TestCharmScheduleRemoval(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	jobUUID := uuid.MustNewUUID().String()
	charmUUID := uuid.MustNewUUID().String()
	when := time.Now().UTC()

	err := st.CharmScheduleRemoval(c.Context(), jobUUID, charmUUID, when)
	c.Assert(err, tc.ErrorIsNil)

	jobs, err := st.GetAllJobs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobs, tc.HasLen, 1)
	c.Check(jobs[0].UUID.String(), tc.Equals, jobUUID)
	c.Check(jobs[0].RemovalType, tc.Equals, removal.CharmJob)
	c.Check(jobs[0].EntityUUID, tc.Equals, charmUUID)
	c.Check(jobs[0].Force, tc.IsFalse)
	c.Check(jobs[0].ScheduledFor, tc.Equals, when)
}

func (s *charmSuite) TestCharmExists(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	charmUUID := s.addCharm(c)

	exists, err := st.CharmExists(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)

	exists, err = st.CharmExists(c.Context(), uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

func (s *charmSuite) TestRescheduleJob(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	jobUUID := uuid.MustNewUUID().String()
	err := st.CharmScheduleRemoval(
		c.Context(), jobUUID, uuid.MustNewUUID().String(), time.Now().UTC(),
	)
	c.Assert(err, tc.ErrorIsNil)

	when := time.Now().UTC().Add(24 * time.Hour)
	err = st.RescheduleJob(c.Context(), jobUUID, when)
	c.Assert(err, tc.ErrorIsNil)

	jobs, err := st.GetAllJobs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobs, tc.HasLen, 1)
	c.Check(jobs[0].ScheduledFor, tc.Equals, when)
}

func (s *charmSuite) TestGetUnusedCharmUUIDs(c *tc.C) {
	// Arrange charms covering each filter dimension of the query.
	now := time.Now().UTC()
	twoHoursAgo := now.Add(-2 * time.Hour)

	// A clean orphan: available, old enough, referenced by nothing.
	orphanUUID := s.insertCharm(c, true, twoHoursAgo)

	// Referenced by an application (the application's own charm).
	appSvc := s.setupApplicationService(c)
	s.createIAASApplication(c, appSvc, "some-app", applicationservice.AddIAASUnitArg{})

	// Referenced by a unit only: repoint the unit at a different charm.
	unitCharmUUID := s.insertCharm(c, true, twoHoursAgo)
	s.repointUnitCharm(c, unitCharmUUID)

	// Not yet available (mid-download placeholder).
	s.insertCharm(c, false, twoHoursAgo)

	// Available and unreferenced, but younger than the cutoff (mid-deploy
	// grace period).
	s.insertCharm(c, true, now)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	charmUUIDs, err := st.GetUnusedCharmUUIDs(c.Context(), now.Add(-time.Hour))

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(charmUUIDs, tc.HasLen, 1)
	c.Check(charmUUIDs[0], tc.Equals, orphanUUID)
}

func (s *charmSuite) TestGetUnusedCharmUUIDsNoRows(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	charmUUIDs, err := st.GetUnusedCharmUUIDs(c.Context(), time.Now().UTC())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(charmUUIDs, tc.HasLen, 0)
}

// insertCharm inserts a charm record with the input availability and
// creation time, returning its UUID.
func (s *charmSuite) insertCharm(c *tc.C, available bool, createTime time.Time) string {
	charmUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, architecture_id, available, create_time)
VALUES (?, ?, 0, ?, ?)`, charmUUID, charmUUID, available, createTime)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil)
	return charmUUID
}

// repointUnitCharm updates the charm of the single unit in the database to
// the input charm UUID.
func (s *charmSuite) repointUnitCharm(c *tc.C, charmUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE unit SET charm_uuid = ?`, charmUUID)
		return errors.Capture(err)
	})
	c.Assert(err, tc.ErrorIsNil)
}
