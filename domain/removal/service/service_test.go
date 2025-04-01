// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corerelation "github.com/juju/juju/core/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	jujutesting.IsolationSuite

	state *MockState
	clock *MockClock
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

func (s *serviceSuite) TestRemoveRelationNoForceSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.RelationAdvanceLife(gomock.Any(), rUUID.String()).Return(nil)
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelation(context.Background(), rUUID, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveRelationForceSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.RelationAdvanceLife(gomock.Any(), rUUID.String()).Return(nil)
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelation(context.Background(), rUUID, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveRelationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	s.state.EXPECT().RelationExists(gomock.Any(), rUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveRelation(context.Background(), rUUID, true)
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.clock = NewMockClock(ctrl)

	return ctrl
}

func (s *serviceSuite) newService(c *gc.C) *Service {
	return &Service{
		st:     s.state,
		clock:  s.clock,
		logger: loggertesting.WrapCheckLog(c),
	}
}

func newRelUUID(c *gc.C) corerelation.UUID {
	rUUID, err := corerelation.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return rUUID
}
