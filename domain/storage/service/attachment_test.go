// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	coreunit "github.com/juju/juju/core/unit"
	domainapplicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// attachmentSuite is a test suite for asserting the parts of the [Service]
// interface that relate to storage attachments.
type attachmentSuite struct {
	state                 *MockState
	storageRegistryGetter *MockModelStorageRegistryGetter
}

// TestAttachmentSuite runs all of the tests contained within [attachmentSuite].
func TestAttachmentSuite(t *testing.T) {
	tc.Run(t, &attachmentSuite{})
}

func (s *attachmentSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)

	c.Cleanup(func() {
		s.state = nil
		s.storageRegistryGetter = nil
	})
	return ctrl
}

// TestGetStorageAttachmentUUIDForStorageInstanceAndUnitInvalidUUIDs asserts
// the various combinations of [coreerrors.NotValid] when supplying invalid
// uuids.
func (s *attachmentSuite) TestGetStorageAttachmentUUIDForStorageInstanceAndUnitInvalidUUIDs(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), s.storageRegistryGetter,
	)

	c.Run("storage instance uuid", func(t *testing.T) {
		_, err := svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
			c.Context(),
			"invalid-storage-id",
			tc.Must(t, coreunit.NewUUID),
		)
		tc.Check(t, err, tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("unit uuid", func(t *testing.T) {
		_, err := svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
			c.Context(),
			tc.Must(t, domainstorage.NewStorageInstanceUUID),
			"invalid-unit-id",
		)
		tc.Check(t, err, tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("both uuids", func(t *testing.T) {
		_, err := svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
			c.Context(),
			"invalid-storage-id",
			"invalid-unit-id",
		)
		tc.Check(t, err, tc.ErrorIs, coreerrors.NotValid)
	})
}

// TestGetStorageAttachmentUUIDForStorageInstanceAndUnitStorageNotFound asserts
// that when getting the storage attachment uuid by storage instance and unit if
// the storage instance doesn't exist in the model the caller MUST get back a
// error satisfying [domainstorageerrors.StorageInstanceNotFound].
func (s *attachmentSuite) TestGetStorageAttachmentUUIDForStorageInstanceAndUnitStorageNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageInstnaceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	stateExp := s.state.EXPECT()
	stateExp.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		gomock.Any(), storageInstnaceUUID, gomock.Any(),
	).Return("", domainstorageerrors.StorageInstanceNotFound)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), s.storageRegistryGetter,
	)
	_, err := svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(),
		storageInstnaceUUID,
		tc.Must(c, coreunit.NewUUID),
	)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestGetStorageAttachmentUUIDForStorageInstanceAndUnitNotFound asserts
// that when getting the storage attachment uuid by storage instance and unit if
// the unit doesn't exist in the model the caller MUST get back a error
// satisfying [domainapplicationerrors.UnitNotFound].
func (s *attachmentSuite) TestGetStorageAttachmentUUIDForStorageInstanceAndUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := tc.Must(c, coreunit.NewUUID)
	stateExp := s.state.EXPECT()
	stateExp.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		gomock.Any(), gomock.Any(), unitUUID,
	).Return("", domainapplicationerrors.UnitNotFound)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), s.storageRegistryGetter,
	)
	_, err := svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(),
		tc.Must(c, domainstorage.NewStorageInstanceUUID),
		unitUUID,
	)
	c.Check(err, tc.ErrorIs, domainapplicationerrors.UnitNotFound)
}

// TestGetStorageAttachmentUUIDForStorageInstanceAndUnit is a happy path test
// for [Service.GetStorageAttachmentUUIDForStorageInstanceAndUnit].
func (s *attachmentSuite) TestGetStorageAttachmentUUIDForStorageInstanceAndUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageAttachmentUUID := tc.Must(c, domainstorageprovisioning.NewStorageAttachmentUUID)
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	stateExp := s.state.EXPECT()
	stateExp.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		gomock.Any(), storageInstanceUUID, unitUUID,
	).Return(storageAttachmentUUID, nil)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), s.storageRegistryGetter,
	)
	uuid, err := svc.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(),
		storageInstanceUUID,
		unitUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, storageAttachmentUUID)
}

// TestGetStorageInstanceAttachmentsInvalidUUID checks that if passed an invalid
// storage instance uuid the service returns a [coreerrors.NotValid] error.
func (s *attachmentSuite) TestGetStorageInstanceAttachmentsInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	badStorageUUID := domainstorage.StorageInstanceUUID("invalid-uuid")
	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), s.storageRegistryGetter,
	)
	_, err := svc.GetStorageInstanceAttachments(c.Context(), badStorageUUID)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetStorageInstanceAttachmentsNotFound checks that if the storage instance
// does not exist the service returns a
// [domainstorageerrors.StorageInstanceNotFound] error.
func (s *attachmentSuite) TestGetStorageInstanceAttachmentsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	siUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	stExp := s.state.EXPECT()
	stExp.GetStorageInstanceAttachments(gomock.Any(), siUUID).Return(
		nil, domainstorageerrors.StorageInstanceNotFound,
	)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), s.storageRegistryGetter,
	)
	_, err := svc.GetStorageInstanceAttachments(c.Context(), siUUID)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

func (s *attachmentSuite) TestGetStorageInstanceAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	siUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	saUUID1 := tc.Must(c, domainstorageprovisioning.NewStorageAttachmentUUID)
	saUUID2 := tc.Must(c, domainstorageprovisioning.NewStorageAttachmentUUID)

	stExp := s.state.EXPECT()
	stExp.GetStorageInstanceAttachments(gomock.Any(), siUUID).Return(
		[]domainstorageprovisioning.StorageAttachmentUUID{
			saUUID1, saUUID2,
		}, nil,
	)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), s.storageRegistryGetter,
	)
	attachments, err := svc.GetStorageInstanceAttachments(c.Context(), siUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(attachments, tc.SameContents, []domainstorageprovisioning.StorageAttachmentUUID{
		saUUID2, saUUID1,
	})
}
