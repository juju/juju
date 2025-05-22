// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	baseSuite
}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestGetAllJobsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dbJobs := []removal.Job{
		{
			UUID:         "job-1",
			RemovalType:  removal.RelationJob,
			EntityUUID:   "rel-1",
			Force:        false,
			ScheduledFor: time.Now().UTC(),
		},
		{
			UUID:         "job-2",
			RemovalType:  removal.RelationJob,
			EntityUUID:   "rel-2",
			Force:        true,
			ScheduledFor: time.Now().UTC().Add(time.Hour),
			Arg: map[string]any{
				"key": "value",
			},
		},
	}

	s.state.EXPECT().GetAllJobs(gomock.Any()).Return(dbJobs, nil)

	jobs, err := s.newService(c).GetAllJobs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobs, tc.DeepEquals, dbJobs)
}

func (s *serviceSuite) TestGetAllJobsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllJobs(gomock.Any()).Return(nil, errors.New("the front fell off"))

	jobs, err := s.newService(c).GetAllJobs(c.Context())
	c.Assert(err, tc.ErrorMatches, "the front fell off")
	c.Check(jobs, tc.IsNil)
}

func (s *serviceSuite) TestExecuteJobUnsupportedType(c *tc.C) {
	var unsupportedJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: unsupportedJobType,
	}

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotSupported)
}
