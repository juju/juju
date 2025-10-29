// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"
	time "time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/rpc/params"
)

// storageRemoveSuite provides a suite of tests for asserting the functionality
// behind removing a storage instance.
type storageRemoveSuite struct {
	baseStorageSuite
}

func TestStorageRemoveSuite(t *testing.T) {
	tc.Run(t, &storageRemoveSuite{})
}

func (s *storageRemoveSuite) TestNegativeMaxWaitTime(c *tc.C) {
	defer s.setupMocks(c).Finish()

	negativeMaxWait := time.Duration(-5)
	res, err := s.api.Remove(c.Context(), params.RemoveStorage{
		Storage: []params.RemoveStorageInstance{
			{
				MaxWait: &negativeMaxWait,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotValid)
}

func (s *storageRemoveSuite) TestRemoveNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(
		gomock.Any(), "data/1",
	).Return("", storageerrors.StorageInstanceNotFound)

	res, err := s.api.Remove(c.Context(), params.RemoveStorage{
		Storage: []params.RemoveStorageInstance{
			{
				Tag: "storage-data/1",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageRemoveSuite) TestRemove(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(
		gomock.Any(), "data/1",
	).Return(storageInstanceUUID, nil)

	removalExp := s.removalService.EXPECT()
	removalExp.RemoveStorageInstance(
		gomock.Any(), storageInstanceUUID, false, time.Duration(0), false,
	).Return(nil)

	res, err := s.api.Remove(c.Context(), params.RemoveStorage{
		Storage: []params.RemoveStorageInstance{
			{
				Tag: "storage-data/1",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *storageRemoveSuite) TestRemoveWithDestroy(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(
		gomock.Any(), "data/1",
	).Return(storageInstanceUUID, nil)

	removalExp := s.removalService.EXPECT()
	removalExp.RemoveStorageInstance(
		gomock.Any(), storageInstanceUUID, false, time.Duration(0), true,
	).Return(nil)

	res, err := s.api.Remove(c.Context(), params.RemoveStorage{
		Storage: []params.RemoveStorageInstance{
			{
				Tag:            "storage-data/1",
				DestroyStorage: true,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *storageRemoveSuite) TestRemoveWithForceAndWait(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(
		gomock.Any(), "data/1",
	).Return(storageInstanceUUID, nil)

	removalExp := s.removalService.EXPECT()
	removalExp.RemoveStorageInstance(
		gomock.Any(), storageInstanceUUID, true, time.Minute, false,
	).Return(nil)

	var (
		force = true
		wait  = time.Minute
	)
	res, err := s.api.Remove(c.Context(), params.RemoveStorage{
		Storage: []params.RemoveStorageInstance{
			{
				Force:   &force,
				MaxWait: &wait,
				Tag:     "storage-data/1",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *storageRemoveSuite) TestRemoveWithStorageAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	attachmentUUID := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)
	attachmentUUIDS := []storageprovisioning.StorageAttachmentUUID{
		attachmentUUID,
	}

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(
		gomock.Any(), "data/1",
	).Return(storageInstanceUUID, nil)
	storageExp.GetStorageAttachmentUUIDsForStorageInstance(gomock.Any(),
		storageInstanceUUID).Return(attachmentUUIDS, nil)

	removalExp := s.removalService.EXPECT()
	removalExp.RemoveStorageAttachmentFromAliveUnit(
		gomock.Any(), attachmentUUID, false, time.Duration(0),
	).Return("", nil)
	removalExp.RemoveStorageInstance(
		gomock.Any(), storageInstanceUUID, false, time.Duration(0), false,
	).Return(nil)

	res, err := s.api.Remove(c.Context(), params.RemoveStorage{
		Storage: []params.RemoveStorageInstance{
			{
				Tag:                "storage-data/1",
				DestroyAttachments: true,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}
