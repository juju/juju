// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	storageprovtesting "github.com/juju/juju/domain/storageprovisioning/testing"
)

type storageSuite struct {
	baseSuite
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestRemoveStorageAttachmentNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := storageprovtesting.GenStorageAttachmentUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.StorageAttachmentExists(gomock.Any(), saUUID.String()).Return(true, nil)
	exp.EnsureStorageAttachmentNotAlive(gomock.Any(), saUUID.String()).Return(nil)
	exp.StorageAttachmentScheduleRemoval(gomock.Any(), gomock.Any(), saUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveStorageAttachment(c.Context(), saUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *storageSuite) TestRemoveStorageAttachmentForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := storageprovtesting.GenStorageAttachmentUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.StorageAttachmentExists(gomock.Any(), saUUID.String()).Return(true, nil)
	exp.EnsureStorageAttachmentNotAlive(gomock.Any(), saUUID.String()).Return(nil)
	exp.StorageAttachmentScheduleRemoval(gomock.Any(), gomock.Any(), saUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveStorageAttachment(c.Context(), saUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *storageSuite) TestRemoveStorageAttachmentForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := storageprovtesting.GenStorageAttachmentUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.modelState.EXPECT()
	exp.StorageAttachmentExists(gomock.Any(), saUUID.String()).Return(true, nil)
	exp.EnsureStorageAttachmentNotAlive(gomock.Any(), saUUID.String()).Return(nil)

	// The first normal removal scheduled immediately.
	exp.StorageAttachmentScheduleRemoval(gomock.Any(), gomock.Any(), saUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.StorageAttachmentScheduleRemoval(gomock.Any(), gomock.Any(), saUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveStorageAttachment(c.Context(), saUUID, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *storageSuite) TestRemoveStorageAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := storageprovtesting.GenStorageAttachmentUUID(c)

	s.modelState.EXPECT().StorageAttachmentExists(gomock.Any(), saUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveStorageAttachment(c.Context(), saUUID, false, 0)
	c.Assert(err, tc.ErrorIs, storageerrors.StorageAttachmentNotFound)
}

func (s *storageSuite) TestExecuteJobForStorageAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageAttachmentJob(c)

	exp := s.modelState.EXPECT()
	exp.GetStorageAttachmentLife(
		gomock.Any(), j.EntityUUID).Return(-1, storageerrors.StorageAttachmentNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForStorageAttachmentStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageAttachmentJob(c)

	s.modelState.EXPECT().GetStorageAttachmentLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestExecuteJobForStorageAttachmentSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageAttachmentJob(c)

	exp := s.modelState.EXPECT()
	exp.GetStorageAttachmentLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.DeleteStorageAttachment(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String())

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newStorageAttachmentJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.StorageAttachmentJob,
		EntityUUID:  storageprovtesting.GenStorageAttachmentUUID(c).String(),
	}
}
