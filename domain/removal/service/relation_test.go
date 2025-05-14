// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
)

type relationSuite struct {
	baseSuite
}

var _ = tc.Suite(&relationSuite{})

func (s *relationSuite) TestRemoveRelationNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.EnsureRelationNotAlive(gomock.Any(), rUUID.String()).Return(nil)
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelation(c.Context(), rUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationSuite) TestRemoveRelationForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.EnsureRelationNotAlive(gomock.Any(), rUUID.String()).Return(nil)
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelation(c.Context(), rUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationSuite) TestRemoveRelationForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.state.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.EnsureRelationNotAlive(gomock.Any(), rUUID.String()).Return(nil)

	// The first normal removal scheduled immediately.
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelation(c.Context(), rUUID, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationSuite) TestRemoveRelationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	s.state.EXPECT().RelationExists(gomock.Any(), rUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveRelation(c.Context(), rUUID, false, 0)
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestProcessRemovalJobInvalidJobType(c *tc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processRelationRemovalJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *relationSuite) TestExecuteJobForRelationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	exp := s.state.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(-1, relationerrors.RelationNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TestExecuteJobForRelationStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	s.state.EXPECT().GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *relationSuite) TestExecuteJobForRelationExistingScopes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	exp := s.state.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return([]string{"unit/0"}, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TestExecuteJobForRelationNoScopes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	exp := s.state.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteRelation(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TestExecuteJobForRelationForceDeletesScopes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)
	j.Force = true

	exp := s.state.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return([]string{"unit/0"}, nil)
	exp.DeleteRelationUnits(c.Context(), j.EntityUUID).Return(nil)
	exp.DeleteRelation(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newRelationJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.RelationJob,
		EntityUUID:  newRelUUID(c).String(),
	}
}

func newRelUUID(c *tc.C) corerelation.UUID {
	rUUID, err := corerelation.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	return rUUID
}
