// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/internal/errors"
)

type relationWithRemoteOffererSuite struct {
	baseSuite
}

func TestRelationWithRemoteOffererSuite(t *testing.T) {
	tc.Run(t, &relationWithRemoteOffererSuite{})
}

func (s *relationWithRemoteOffererSuite) TestRemoveRelationWithRemoteOffererNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relUUID := tc.Must(c, relation.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.RelationWithRemoteOffererExists(gomock.Any(), relUUID.String()).Return(true, nil)
	exp.EnsureRelationWithRemoteOffererNotAliveCascade(gomock.Any(), relUUID.String()).Return(internal.CascadedRelationWithRemoteOffererLives{}, nil)
	exp.RelationWithRemoteOffererScheduleRemoval(gomock.Any(), gomock.Any(), relUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelationWithRemoteOfferer(c.Context(), relUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationWithRemoteOffererSuite) TestRemoveRelationWithRemoteOffererForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relUUID := tc.Must(c, relation.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.RelationWithRemoteOffererExists(gomock.Any(), relUUID.String()).Return(true, nil)
	exp.EnsureRelationWithRemoteOffererNotAliveCascade(gomock.Any(), relUUID.String()).Return(internal.CascadedRelationWithRemoteOffererLives{}, nil)
	exp.RelationWithRemoteOffererScheduleRemoval(gomock.Any(), gomock.Any(), relUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelationWithRemoteOfferer(c.Context(), relUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationWithRemoteOffererSuite) TestRemoveRelationWithRemoteOffererForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relUUID := tc.Must(c, relation.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.modelState.EXPECT()
	exp.RelationWithRemoteOffererExists(gomock.Any(), relUUID.String()).Return(true, nil)
	exp.EnsureRelationWithRemoteOffererNotAliveCascade(gomock.Any(), relUUID.String()).Return(internal.CascadedRelationWithRemoteOffererLives{}, nil)

	// The first normal removal scheduled immediately.
	exp.RelationWithRemoteOffererScheduleRemoval(gomock.Any(), gomock.Any(), relUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.RelationWithRemoteOffererScheduleRemoval(gomock.Any(), gomock.Any(), relUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelationWithRemoteOfferer(c.Context(), relUUID, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationWithRemoteOffererSuite) TestRemoveRelationWithRemoteOffererDepartedSyntheticUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relUUID := tc.Must(c, relation.NewUUID)
	relUnitUUID1 := tc.Must(c, relation.NewUnitUUID)
	relUnitUUID2 := tc.Must(c, relation.NewUnitUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.modelState.EXPECT()
	exp.RelationWithRemoteOffererExists(gomock.Any(), relUUID.String()).Return(true, nil)
	exp.EnsureRelationWithRemoteOffererNotAliveCascade(gomock.Any(), relUUID.String()).Return(internal.CascadedRelationWithRemoteOffererLives{
		SyntheticRelationUnitUUIDs: []string{relUnitUUID1.String(), relUnitUUID2.String()},
	}, nil)
	exp.LeaveScope(gomock.Any(), relUnitUUID1.String()).Return(nil)
	exp.LeaveScope(gomock.Any(), relUnitUUID2.String()).Return(nil)
	exp.RelationWithRemoteOffererScheduleRemoval(gomock.Any(), gomock.Any(), relUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelationWithRemoteOfferer(c.Context(), relUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationWithRemoteOffererSuite) TestRemoveRelationWithRemoteOffererNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relUUID := tc.Must(c, relation.NewUUID)

	s.modelState.EXPECT().RelationWithRemoteOffererExists(gomock.Any(), relUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveRelationWithRemoteOfferer(c.Context(), relUUID, false, 0)
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationWithRemoteOffererSuite) TestProcessRelationWithRemoteOffererRemovalJobInvalidJobType(c *tc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processRelationWithRemoteOffererRemovalJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *relationWithRemoteOffererSuite) TestExecuteJobForRelationWithRemoteOffererNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationWithRemoteOffererJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(-1, relationerrors.RelationNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationWithRemoteOffererSuite) TestExecuteJobForRelationWithRemoteOffererError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationWithRemoteOffererJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(-1, errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *relationWithRemoteOffererSuite) TestExecuteJobForRelationWithRemoteOffererStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationWithRemoteOffererJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *relationWithRemoteOffererSuite) TestExecuteJobForRelationWithRemoteOffererUnitsInScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationWithRemoteOffererJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return([]string{"unit/0"}, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationWithRemoteOffererSuite) TestExecuteJobForRelationWithRemoteOffererUnitsInScopeForce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationWithRemoteOffererJob(c)
	j.Force = true

	exp := s.modelState.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return([]string{"unit/0"}, nil)
	exp.DeleteRelationUnits(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteRelationWithRemoteOfferer(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationWithRemoteOffererSuite) TestExecuteJobForRelationWithRemoteOffererDyingDeleteRelationWithRemoteOfferer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationWithRemoteOffererJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteRelationWithRemoteOfferer(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationWithRemoteOffererSuite) TestExecuteJobForRelationWithRemoteOffererDyingDeleteRelationWithRemoteOffererError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationWithRemoteOffererJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return(nil, nil)
	exp.DeleteRelationWithRemoteOfferer(gomock.Any(), j.EntityUUID).Return(errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func newRelationWithRemoteOffererJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.RelationWithRemoteOffererJob,
		EntityUUID:  tc.Must(c, relation.NewUUID).String(),
	}
}
