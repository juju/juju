// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	baseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestGetAllJobsSuccess(c *gc.C) {
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

	jobs, err := s.newService(c).GetAllJobs(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jobs, jc.DeepEquals, dbJobs)
}

func (s *serviceSuite) TestGetAllJobsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllJobs(gomock.Any()).Return(nil, errors.New("the front fell off"))

	jobs, err := s.newService(c).GetAllJobs(context.Background())
	c.Assert(err, gc.ErrorMatches, "the front fell off")
	c.Check(jobs, gc.IsNil)
}

func (s *serviceSuite) TestExecuteJobUnsupportedType(c *gc.C) {
	var unsupportedJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: unsupportedJobType,
	}

	err := s.newService(c).ExecuteJob(context.Background(), job)
	c.Check(err, jc.ErrorIs, removalerrors.RemovalJobTypeNotSupported)
}
