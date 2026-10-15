// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	relationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
)

type relationSuite struct {
	baseSuite
}

func TestRelationSuite(t *testing.T) {
	tc.Run(t, &relationSuite{})
}

func (s *relationSuite) TestRemoveRelationNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := relationtesting.GenRelationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.EnsureRelationNotAlive(gomock.Any(), rUUID.String()).Return(nil)
	exp.GetRelationKind(gomock.Any(), rUUID.String()).Return(internal.RelationKindOrdinary, nil)
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelation(c.Context(), rUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationSuite) TestRemoveRelationForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := relationtesting.GenRelationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.EnsureRelationNotAlive(gomock.Any(), rUUID.String()).Return(nil)
	exp.GetRelationKind(gomock.Any(), rUUID.String()).Return(internal.RelationKindOrdinary, nil)
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveRelation(c.Context(), rUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationSuite) TestRemoveRelationForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := relationtesting.GenRelationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.modelState.EXPECT()
	exp.RelationExists(gomock.Any(), rUUID.String()).Return(true, nil)
	exp.EnsureRelationNotAlive(gomock.Any(), rUUID.String()).Return(nil)
	exp.GetRelationKind(gomock.Any(), rUUID.String()).Return(internal.RelationKindOrdinary, nil).Times(2)

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

	rUUID := relationtesting.GenRelationUUID(c)
	s.modelState.EXPECT().RelationExists(gomock.Any(), rUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveRelation(c.Context(), rUUID, false, 0)
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestRelationScheduleRemovalOrdinaryRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := relationtesting.GenRelationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.GetRelationKind(gomock.Any(), rUUID.String()).Return(internal.RelationKindOrdinary, nil)
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).relationScheduleRemoval(c.Context(), rUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationSuite) TestRelationScheduleRemovalRelationWithRemoteOfferer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := relationtesting.GenRelationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	// Cross-model relations are routed to the remote offerer removal
	// path: the plain relation removal job cannot delete their CMR
	// children.
	exp := s.modelState.EXPECT()
	exp.GetRelationKind(gomock.Any(), rUUID.String()).Return(internal.RelationKindWithRemoteOfferer, nil)
	exp.RelationWithRemoteOffererScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).relationScheduleRemoval(c.Context(), rUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationSuite) TestRelationScheduleRemovalRelationWithRemoteConsumer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := relationtesting.GenRelationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	// Cross-model relations are routed to the remote consumer removal
	// path: the plain relation removal job cannot delete their CMR
	// children.
	exp := s.modelState.EXPECT()
	exp.GetRelationKind(gomock.Any(), rUUID.String()).Return(internal.RelationKindWithRemoteConsumer, nil)
	exp.RelationWithRemoteConsumerScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).relationScheduleRemoval(c.Context(), rUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationSuite) TestRelationScheduleRemovalAlreadyRemovedRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := relationtesting.GenRelationUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	// A relation removed by another process is scheduled on the plain
	// relation removal path anyway: the job executor sees the relation
	// is gone and deletes the job.
	exp := s.modelState.EXPECT()
	exp.GetRelationKind(gomock.Any(), rUUID.String()).Return(internal.RelationKindOrdinary, relationerrors.RelationNotFound)
	exp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), rUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).relationScheduleRemoval(c.Context(), rUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *relationSuite) TestRelationScheduleRemovalKindError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rUUID := relationtesting.GenRelationUUID(c)

	s.modelState.EXPECT().GetRelationKind(gomock.Any(), rUUID.String()).Return(
		internal.RelationKindOrdinary, errors.New("front fell off"))

	_, err := s.newService(c).relationScheduleRemoval(c.Context(), rUUID, false, 0)
	c.Assert(err, tc.ErrorMatches, ".*front fell off")
}

func (s *relationSuite) TestLeaveScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUnitUUID := relationtesting.GenRelationUnitUUID(c)
	s.modelState.EXPECT().LeaveScope(gomock.Any(), relationUnitUUID.String()).Return(nil)

	err := s.newService(c).LeaveScope(c.Context(), relationUnitUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TestLeaveScopeRelationUnitNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.newService(c).LeaveScope(c.Context(), "bad-relation-unit-uuid")
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationSuite) TestExecuteJobForRelationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(-1, relationerrors.RelationNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TestProcessRelationRemovalJobInvalidJobType(c *tc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processRelationRemovalJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *relationSuite) TestExecuteJobForRelationStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	s.modelState.EXPECT().GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *relationSuite) TestExecuteJobForRelationExistingScopes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return([]string{"unit/0"}, nil)
	exp.IsUnitDyingAndBlocked(gomock.All(), "unit/0").Return(false, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TestExecuteJobForRelationExistingScopesMultipleUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return([]string{"unit/0", "unit/1"}, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TestExecuteJobForRelationExistingScopesUnitInError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	exp := s.modelState.EXPECT()
	exp.GetRelationLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.UnitNamesInScope(gomock.Any(), j.EntityUUID).Return([]string{"unit/0"}, nil)
	exp.IsUnitDyingAndBlocked(gomock.All(), "unit/0").Return(true, nil)
	exp.DeleteRelationUnits(c.Context(), j.EntityUUID).Return(nil)
	exp.DeleteRelation(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TestExecuteJobForRelationNoScopes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newRelationJob(c)

	exp := s.modelState.EXPECT()
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

	exp := s.modelState.EXPECT()
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
		EntityUUID:  relationtesting.GenRelationUUID(c).String(),
	}
}
