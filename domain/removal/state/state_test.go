// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/removal"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestGetAllJobsNoRows(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	jobs, err := st.GetAllJobs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(jobs, tc.HasLen, 0)
}

func (s *stateSuite) TestGetAllJobsWithData(c *tc.C) {
	ins := `
INSERT INTO removal (uuid, removal_type_id, entity_uuid, force, scheduled_for, arg) 
VALUES (?, ?, ?, ?, ?, ?)`

	jID1, _ := removal.NewUUID()
	jID2, _ := removal.NewUUID()
	now := time.Now().UTC()

	_, err := s.DB().Exec(ins, jID1, 0, "rel-1", 0, now, nil)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(ins, jID2, 0, "rel-2", 1, now, `{"special-key":"special-value"}`)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	jobs, err := st.GetAllJobs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobs, tc.HasLen, 2)

	c.Check(jobs[0], tc.DeepEquals, removal.Job{
		UUID:         jID1,
		RemovalType:  removal.RelationJob,
		EntityUUID:   "rel-1",
		Force:        false,
		ScheduledFor: now,
	})

	c.Check(jobs[1], tc.DeepEquals, removal.Job{
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

func (s *stateSuite) TestDeleteJob(c *tc.C) {
	ins := `
INSERT INTO removal (uuid, removal_type_id, entity_uuid, force, scheduled_for, arg) 
VALUES (?, ?, ?, ?, ?, ?)`

	jID1, _ := removal.NewUUID()
	now := time.Now().UTC()
	_, err := s.DB().Exec(ins, jID1, 0, "rel-1", 0, now, nil)
	c.Assert(err, tc.ErrorIsNil)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteJob(c.Context(), jID1.String())
	c.Assert(err, tc.ErrorIsNil)

	row := s.DB().QueryRow("SELECT count(*) FROM removal where uuid = ?", jID1)
	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	// Idempotent.
	err = st.DeleteJob(c.Context(), jID1.String())
	c.Assert(err, tc.ErrorIsNil)
}
