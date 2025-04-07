// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
)

type relationSuite struct {
	baseSuite
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) TestRemoveRelationNoForceSuccess(c *gc.C) {
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

func (s *relationSuite) TestRemoveRelationForceSuccess(c *gc.C) {
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

func (s *relationSuite) TestRemoveRelationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := newRelUUID(c)

	s.state.EXPECT().RelationExists(gomock.Any(), rUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveRelation(context.Background(), rUUID, true)
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestProcessRemovalJobInvalidJobType(c *gc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processRelationRemovalJob(context.Background(), job)
	c.Check(err, jc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *relationSuite) TestExecuteJobForRelationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	exp := s.state.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(-1, relationerrors.RelationNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(context.Background(), j)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationSuite) TestExecuteJobForRelationStillAlive(c *gc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	s.state.EXPECT().GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(context.Background(), j)
	c.Assert(err, jc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *relationSuite) TestExecuteJobForRelationExistingScopes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	exp := s.state.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return([]string{"unit/0"}, nil)

	err := s.newService(c).ExecuteJob(context.Background(), j)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationSuite) TestExecuteJobForRelationNoScopes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	exp := s.state.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteRelation(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(context.Background(), j)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationSuite) TestExecuteJobForRelationForceDeletesScopes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)
	j.Force = true

	exp := s.state.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return([]string{"unit/0"}, nil)
	exp.DeleteRelationUnits(context.Background(), j.EntityUUID).Return(nil)
	exp.DeleteRelation(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(context.Background(), j)
	c.Assert(err, jc.ErrorIsNil)
}

func newRelationJob(c *gc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.RelationJob,
		EntityUUID:  newRelUUID(c).String(),
	}
}

func newRelUUID(c *gc.C) corerelation.UUID {
	rUUID, err := corerelation.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return rUUID
}
