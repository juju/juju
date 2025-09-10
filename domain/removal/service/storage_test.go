// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
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
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.StorageAttachmentNotFound)
}
