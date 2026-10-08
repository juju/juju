// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreresourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

type resourceSuite struct {
	baseSuite
}

func TestResourceSuite(t *testing.T) {
	tc.Run(t, &resourceSuite{})
}

func (s *resourceSuite) TestProcessResourceRemovalJobInvalidJobType(c *tc.C) {
	job := removal.Job{RemovalType: 500}
	err := s.newService(c).processResourceRemovalJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *resourceSuite) TestExecuteResourceRemovalJob(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newResourceJob(c)
	s.modelState.EXPECT().ResourceExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	s.modelState.EXPECT().DeleteResourceIfUnused(gomock.Any(), job.EntityUUID).Return(true, nil)
	s.modelState.EXPECT().DeleteJob(gomock.Any(), job.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceSuite) TestExecuteResourceRemovalJobResourceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newResourceJob(c)
	s.modelState.EXPECT().ResourceExists(gomock.Any(), job.EntityUUID).Return(false, nil)
	s.modelState.EXPECT().DeleteJob(gomock.Any(), job.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceSuite) TestExecuteResourceRemovalJobIncomplete(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newResourceJob(c)
	s.modelState.EXPECT().ResourceExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	s.modelState.EXPECT().DeleteResourceIfUnused(gomock.Any(), job.EntityUUID).Return(false, nil)

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceSuite) TestExecuteResourceRemovalJobExistsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newResourceJob(c)
	s.modelState.EXPECT().ResourceExists(gomock.Any(), job.EntityUUID).
		Return(false, errors.Errorf("boom"))

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Check(err, tc.ErrorMatches, `checking if resource .* exists: boom`)
}

func (s *resourceSuite) TestExecuteResourceRemovalJobError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	job := newResourceJob(c)
	s.modelState.EXPECT().ResourceExists(gomock.Any(), job.EntityUUID).Return(true, nil)
	s.modelState.EXPECT().DeleteResourceIfUnused(gomock.Any(), job.EntityUUID).
		Return(false, errors.Errorf("boom"))

	err := s.newService(c).ExecuteJob(c.Context(), job)
	c.Check(err, tc.ErrorMatches, `deleting resource .*: boom`)
}

func newResourceJob(c *tc.C) removal.Job {
	return removal.Job{
		UUID:        tc.Must(c, removal.NewUUID),
		RemovalType: removal.ResourceJob,
		EntityUUID:  coreresourcetesting.GenResourceUUID(c).String(),
	}
}
