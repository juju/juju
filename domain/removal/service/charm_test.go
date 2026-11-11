// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

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
	exp.CharmHasApplicationReferences(gomock.Any(), job.EntityUUID).Return(false, nil)
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

func (s *charmSuite) TestExecuteCharmRemovalJobApplicationReferenced(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// An application still references the charm, so the charm is retained and
	// the job completes without attempting resource or charm deletion.
	job := newCharmJob(c)
	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.CharmHasApplicationReferences(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.DeleteJob(gomock.Any(), job.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestExecuteCharmRemovalJobUnitReferenced(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// A unit can still hold its charm foreign key after it becomes dying. Keep
	// the job for a later pass instead of deleting it before the unit row goes.
	job := newCharmJob(c)
	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.CharmHasApplicationReferences(gomock.Any(), job.EntityUUID).Return(false, nil)
	exp.DeleteOrphanedResources(gomock.Any(), job.EntityUUID).Return(nil)
	exp.CharmHasReferencedResources(gomock.Any(), job.EntityUUID).Return(false, nil)
	exp.DeleteCharmIfUnused(gomock.Any(), job.EntityUUID).
		Return(errors.Errorf("unit still references charm").Add(removalerrors.RemovalJobIncomplete))

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestExecuteCharmRemovalJobApplicationReferencesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newCharmJob(c)
	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.CharmHasApplicationReferences(gomock.Any(), job.EntityUUID).
		Return(false, errors.Errorf("boom"))

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Check(err, tc.ErrorMatches, `checking application references for charm .* boom`)
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
	exp.CharmHasApplicationReferences(gomock.Any(), job.EntityUUID).Return(false, nil)
	exp.DeleteOrphanedResources(gomock.Any(), job.EntityUUID).Return(errors.Errorf("boom"))

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Check(err, tc.ErrorMatches, `deleting orphaned resources for charm .* boom`)
}

func (s *charmSuite) TestExecuteCharmRemovalJobPinned(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newCharmJob(c)
	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.CharmHasApplicationReferences(gomock.Any(), job.EntityUUID).Return(false, nil)
	exp.DeleteOrphanedResources(gomock.Any(), job.EntityUUID).Return(nil)
	exp.CharmHasReferencedResources(gomock.Any(), job.EntityUUID).Return(true, nil)

	// The charm is pinned, so the job remains incomplete: it stays in the
	// table for retry at the removal worker's polling interval, and is not
	// deleted.
	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmSuite) TestExecuteCharmRemovalJobDeleteError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newCharmJob(c)
	exp := s.modelState.EXPECT()
	exp.CharmExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	exp.CharmHasApplicationReferences(gomock.Any(), job.EntityUUID).Return(false, nil)
	exp.DeleteOrphanedResources(gomock.Any(), job.EntityUUID).Return(nil)
	exp.CharmHasReferencedResources(gomock.Any(), job.EntityUUID).Return(false, nil)
	exp.DeleteCharmIfUnused(gomock.Any(), job.EntityUUID).Return(errors.Errorf("boom"))

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Check(err, tc.ErrorMatches, `deleting charm .* boom`)
}

func newCharmJob(c *tc.C) removal.Job {
	return removal.Job{
		UUID:        tc.Must(c, removal.NewUUID),
		RemovalType: removal.CharmJob,
		EntityUUID:  corecharmtesting.GenCharmID(c).String(),
	}
}
