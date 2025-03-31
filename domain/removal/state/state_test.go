// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/removal"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestGetAllJobsNoRows(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	jobs, err := st.GetAllJobs(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(jobs, gc.HasLen, 0)
}

func (s *stateSuite) TestGetAllJobsWithData(c *gc.C) {
	ins := `
INSERT INTO removal (uuid, removal_type_id, entity_uuid, force, scheduled_for, arg) 
VALUES (?, ?, ?, ?, ?, ?)`

	jID1, _ := removal.NewUUID()
	jID2, _ := removal.NewUUID()
	now := time.Now().UTC()

	_, err := s.DB().Exec(ins, jID1, 0, "rel-1", 0, now, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.DB().Exec(ins, jID2, 0, "rel-2", 1, now, `{"special-key":"special-value"}`)
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	jobs, err := st.GetAllJobs(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jobs, gc.HasLen, 2)

	c.Check(jobs[0], jc.DeepEquals, removal.Job{
		UUID:         jID1,
		RemovalType:  removal.RelationJob,
		EntityUUID:   "rel-1",
		Force:        false,
		ScheduledFor: now,
	})

	c.Check(jobs[1], jc.DeepEquals, removal.Job{
		UUID:         jID2,
		RemovalType:  removal.RelationJob,
		EntityUUID:   "rel-2",
		Force:        true,
		ScheduledFor: now,
		Arg: map[string]any{
			"special-key": "special-value",
		},
	})
}

func (s *stateSuite) TestRelationExists(c *gc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 0, "some-relation-id")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationExists(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, gc.Equals, true)

	exists, err = st.RelationExists(context.Background(), "not-today-henry")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, gc.Equals, false)
}

func (s *stateSuite) TestRelationAdvanceLifeNormalSuccess(c *gc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 0, "some-relation-id")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.RelationAdvanceLife(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIsNil)

	// Relation had life "alive" and should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM relation where uuid = ?", "some-relation-uuid")
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(lifeID, gc.Equals, 1)
}

func (s *stateSuite) TestRelationAdvanceLifeDyingSuccess(c *gc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 1, "some-relation-id")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.RelationAdvanceLife(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIsNil)

	// Relation was already "dying" and should be unchanged.
	row := s.DB().QueryRow("SELECT life_id FROM relation where uuid = ?", "some-relation-uuid")
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(lifeID, gc.Equals, 1)
}

func (s *stateSuite) TestRelationAdvanceLifeNotExistsSuccess(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	err := st.RelationAdvanceLife(context.Background(), "some-relation-uuid")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestRelationRemovalNormalSuccess(c *gc.C) {
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

	c.Check(removalTypeID, gc.Equals, 0)
	c.Check(rUUID, gc.Equals, "some-relation-uuid")
	c.Check(force, gc.Equals, false)
	c.Check(scheduledFor, gc.Equals, when)
}
func (s *stateSuite) TestRelationRemovalNotExistsSuccess(c *gc.C) {
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

	c.Check(removalType, gc.Equals, "relation")
	c.Check(rUUID, gc.Equals, "some-relation-uuid")
	c.Check(force, gc.Equals, true)
	c.Check(scheduledFor, gc.Equals, when)
}
