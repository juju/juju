// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/mattn/go-sqlite3"

	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type charmSuite struct {
	baseSuite
}

func TestCharmSuite(t *testing.T) {
	tc.Run(t, &charmSuite{})
}

func (s *charmSuite) TestProcessCharmRemovalJobInvalidType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newCharmJob(c)
	j.RemovalType = removal.ApplicationJob

	err := s.newService(c).processCharmRemovalJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *charmSuite) TestProcessCharmRemovalJobCharmAlreadyRemoved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newCharmJob(c)

	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), j.EntityUUID).Return(false, nil)

	err := s.newService(c).processCharmRemovalJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestProcessCharmRemovalJobSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newCharmJob(c)

	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), j.EntityUUID).Return(nil)
	// A nil return also covers the charm still being in use; either way the
	// job is done and will be deleted.
	exp.DeleteCharmIfUnused(gomock.Any(), j.EntityUUID).Return(nil)

	err := s.newService(c).processCharmRemovalJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestProcessCharmRemovalJobForeignKeyConstraintReschedules(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newCharmJob(c)
	now := time.Now().UTC()

	fkErr := sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintForeignKey}

	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), j.EntityUUID).Return(fkErr)
	exp.RescheduleJob(gomock.Any(), j.UUID.String(), now.Add(charmRemovalRescheduleDelay)).Return(nil)

	s.clock.EXPECT().Now().Return(now)

	err := s.newService(c).processCharmRemovalJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)
}

func (s *charmSuite) TestProcessCharmRemovalJobForeignKeyConstraintRescheduleFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newCharmJob(c)
	now := time.Now().UTC()

	fkErr := sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintForeignKey}

	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), j.EntityUUID).Return(fkErr)
	exp.RescheduleJob(gomock.Any(), j.UUID.String(), gomock.Any()).Return(errors.Errorf("the front fell off"))

	s.clock.EXPECT().Now().Return(now)

	err := s.newService(c).processCharmRemovalJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *charmSuite) TestProcessCharmRemovalJobDeleteCharmError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newCharmJob(c)

	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), j.EntityUUID).Return(errors.Errorf("the front fell off"))

	err := s.newService(c).processCharmRemovalJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *charmSuite) TestProcessCharmRemovalJobDeleteOrphanedResourcesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newCharmJob(c)

	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), j.EntityUUID).Return(errors.Errorf("the front fell off"))

	err := s.newService(c).processCharmRemovalJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *charmSuite) TestProcessCharmRemovalJobCharmExistsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newCharmJob(c)

	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), j.EntityUUID).Return(false, errors.Errorf("the front fell off"))

	err := s.newService(c).processCharmRemovalJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *charmSuite) TestExecuteJobForCharmDeletesJobOnSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newCharmJob(c)

	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestExecuteJobForCharmIncompleteKeepsJob(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newCharmJob(c)
	now := time.Now().UTC()

	fkErr := sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintForeignKey}

	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteCharmIfUnused(gomock.Any(), j.EntityUUID).Return(fkErr)
	exp.RescheduleJob(gomock.Any(), j.UUID.String(), gomock.Any()).Return(nil)
	// No DeleteJob expectation: the job row is kept for the rescheduled time.

	s.clock.EXPECT().Now().Return(now)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestScheduleCharmRemoval(c *tc.C) {
	defer s.setupMocks(c).Finish()

	charmUUID := uuid.MustNewUUID().String()
	now := time.Now().UTC()

	s.clock.EXPECT().Now().Return(now)
	s.modelState.EXPECT().CharmScheduleRemoval(gomock.Any(), gomock.Any(), charmUUID, now).Return(nil)

	err := s.newService(c).ScheduleCharmRemoval(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestScheduleCharmRemovalError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	charmUUID := uuid.MustNewUUID().String()
	now := time.Now().UTC()

	s.clock.EXPECT().Now().Return(now)
	s.modelState.EXPECT().CharmScheduleRemoval(gomock.Any(), gomock.Any(), charmUUID, now).
		Return(errors.Errorf("the front fell off"))

	err := s.newService(c).ScheduleCharmRemoval(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *charmSuite) TestScheduleCharmRemovalsForUnusedCharms(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC()
	pendingUUID := uuid.MustNewUUID().String()
	orphanUUIDs := []string{
		uuid.MustNewUUID().String(),
		pendingUUID,
		uuid.MustNewUUID().String(),
	}

	s.clock.EXPECT().Now().Return(now).Times(3)

	exp := s.modelState.EXPECT()
	exp.GetUnusedCharmUUIDs(gomock.Any(), now.Add(-charmOrphanGracePeriod)).Return(orphanUUIDs, nil)
	exp.GetAllJobs(gomock.Any()).Return([]removal.Job{{
		UUID:        removal.UUID(uuid.MustNewUUID().String()),
		RemovalType: removal.CharmJob,
		EntityUUID:  pendingUUID,
	}}, nil)
	// The charm with a pending job is skipped; jobs are scheduled for the
	// other two.
	exp.CharmScheduleRemoval(gomock.Any(), gomock.Any(), orphanUUIDs[0], now).Return(nil)
	exp.CharmScheduleRemoval(gomock.Any(), gomock.Any(), orphanUUIDs[2], now).Return(nil)

	err := s.newService(c).ScheduleCharmRemovalsForUnusedCharms(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestScheduleCharmRemovalsForUnusedCharmsNone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC()

	s.clock.EXPECT().Now().Return(now)
	s.modelState.EXPECT().GetUnusedCharmUUIDs(gomock.Any(), now.Add(-charmOrphanGracePeriod)).Return(nil, nil)
	// No GetAllJobs call when there are no unused charms.

	err := s.newService(c).ScheduleCharmRemovalsForUnusedCharms(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestScheduleCharmRemovalsForUnusedCharmsGetUnusedError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC()

	s.clock.EXPECT().Now().Return(now)
	s.modelState.EXPECT().GetUnusedCharmUUIDs(gomock.Any(), gomock.Any()).
		Return(nil, errors.Errorf("the front fell off"))

	err := s.newService(c).ScheduleCharmRemovalsForUnusedCharms(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func newCharmJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.CharmJob,
		EntityUUID:  uuid.MustNewUUID().String(),
	}
}
