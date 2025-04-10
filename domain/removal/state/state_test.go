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

func (s *stateSuite) TestDeleteJob(c *gc.C) {
	ins := `
INSERT INTO removal (uuid, removal_type_id, entity_uuid, force, scheduled_for, arg) 
VALUES (?, ?, ?, ?, ?, ?)`

	jID1, _ := removal.NewUUID()
	now := time.Now().UTC()
	_, err := s.DB().Exec(ins, jID1, 0, "rel-1", 0, now, nil)
	c.Assert(err, jc.ErrorIsNil)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteJob(context.Background(), jID1.String())
	c.Assert(err, jc.ErrorIsNil)

	row := s.DB().QueryRow("SELECT count(*) FROM removal where uuid = ?", jID1)
	var count int
	err = row.Scan(&count)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, 0)

	// Idempotent.
	err = st.DeleteJob(context.Background(), jID1.String())
	c.Assert(err, jc.ErrorIsNil)
}
