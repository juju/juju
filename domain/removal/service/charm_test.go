// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	corecharmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

type charmSuite struct {
	baseSuite
}

func TestCharmSuite(t *testing.T) {
	tc.Run(t, &charmSuite{})
}

func (s *charmSuite) TestProcessCharmRemovalJobInvalidJobType(c *tc.C) {
	job := removal.Job{RemovalType: 500}
	err := s.newService(c).processCharmRemovalJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *charmSuite) TestExecuteCharmRemovalJob(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newCharmJob(c)
	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), job.EntityUUID).Return(nil)
	exp.CharmHasReferencedResources(gomock.Any(), job.EntityUUID).Return(false, nil)
	exp.DeleteCharmIfUnused(gomock.Any(), job.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), job.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestExecuteCharmRemovalJobCharmNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newCharmJob(c)
	s.modelState.EXPECT().CharmExists(gomock.Any(), job.EntityUUID).Return(false, nil)
	s.modelState.EXPECT().DeleteJob(gomock.Any(), job.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestExecuteCharmRemovalJobInUse(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// The charm is still referenced, so DeleteCharmIfUnused leaves it in
	// place. The job is deleted; the removal hooks schedule a new job when
	// the last reference drops.
	job := newCharmJob(c)
	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), job.EntityUUID).Return(nil)
	exp.CharmHasReferencedResources(gomock.Any(), job.EntityUUID).Return(false, nil)
	exp.DeleteCharmIfUnused(gomock.Any(), job.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), job.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestExecuteCharmRemovalJobExistsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newCharmJob(c)
	s.modelState.EXPECT().CharmExists(gomock.Any(), job.EntityUUID).
		Return(false, errors.Errorf("boom"))

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Check(err, tc.ErrorMatches, `checking if charm .* exists: boom`)
}

func (s *charmSuite) TestExecuteCharmRemovalJobDeleteOrphanedResourcesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newCharmJob(c)
	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), job.EntityUUID).Return(errors.Errorf("boom"))

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Check(err, tc.ErrorMatches, `deleting orphaned resources for charm .* boom`)
}

func (s *charmSuite) TestExecuteCharmRemovalJobPinnedRescheduled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	when := time.Now().UTC()

	job := newCharmJob(c)
	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), job.EntityUUID).Return(nil)
	exp.CharmHasReferencedResources(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.RescheduleJob(gomock.Any(), job.UUID.String(), when.Add(rescheduleDelay)).Return(nil)
	s.clock.EXPECT().Now().Return(when)

	// The job is rescheduled instead of deleted: it stays in the table, not
	// due for rescheduleDelay, so ExecuteJob completes without error and
	// without deleting the job.
	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestExecuteCharmRemovalJobPinnedRescheduleError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newCharmJob(c)
	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), job.EntityUUID).Return(nil)
	exp.CharmHasReferencedResources(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.RescheduleJob(gomock.Any(), job.UUID.String(), gomock.Any()).Return(errors.Errorf("boom"))
	s.clock.EXPECT().Now().Return(time.Now().UTC())

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Check(err, tc.ErrorMatches, `rescheduling charm removal job .* boom`)
}

func (s *charmSuite) TestExecuteCharmRemovalJobDeleteError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newCharmJob(c)
	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.DeleteOrphanedResources(gomock.Any(), job.EntityUUID).Return(nil)
	exp.CharmHasReferencedResources(gomock.Any(), job.EntityUUID).Return(false, nil)
	exp.DeleteCharmIfUnused(gomock.Any(), job.EntityUUID).Return(errors.Errorf("boom"))

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Check(err, tc.ErrorMatches, `deleting charm .* boom`)
}

func (s *charmSuite) TestScheduleCharmRemoval(c *tc.C) {
	defer s.setupMocks(c).Finish()

	charmUUID := corecharmtesting.GenCharmID(c)
	when := time.Now().UTC()

	s.clock.EXPECT().Now().Return(when)
	s.modelState.EXPECT().CharmScheduleRemoval(
		gomock.Any(), gomock.Any(), charmUUID.String(), when,
	).Return(nil)

	err := s.newService(c).ScheduleCharmRemoval(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestScheduleCharmRemovalInvalidCharmUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.newService(c).ScheduleCharmRemoval(c.Context(), "not-a-uuid")
	c.Check(err, tc.ErrorMatches, `validating charm UUID: .*`)
}

func newCharmJob(c *tc.C) removal.Job {
	return removal.Job{
		UUID:        tc.Must(c, removal.NewUUID),
		RemovalType: removal.CharmJob,
		EntityUUID:  corecharmtesting.GenCharmID(c).String(),
	}
}
