// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	stderrors "errors"
	"testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	coreunit "github.com/juju/juju/core/unit"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/rpc/params"
)

// storageRemovalSuite tests [common.ClassifyStorageRemoval].
type storageRemovalSuite struct {
	storageService *mocks.MockStorageRemovalClassifier
}

// TestStorageRemovalSuite runs all tests contained within
// [storageRemovalSuite].
func TestStorageRemovalSuite(t *testing.T) {
	tc.Run(t, &storageRemovalSuite{})
}

func (s *storageRemovalSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.storageService = mocks.NewMockStorageRemovalClassifier(ctrl)
	c.Cleanup(func() {
		s.storageService = nil
	})
	return ctrl
}

func (s *storageRemovalSuite) newInstance(c *tc.C, id string, persistent bool) domainstorage.StorageInstanceInfo {
	return domainstorage.StorageInstanceInfo{
		ID:         id,
		Persistent: persistent,
		UUID:       tc.Must(c, domainstorage.NewStorageInstanceUUID),
	}
}

// TestClassifyStorageRemovalNoUnits asserts that an empty unit list results in
// empty destroyed and detached storage lists.
func (s *storageRemovalSuite) TestClassifyStorageRemovalNoUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	destroyed, detached, err := common.ClassifyStorageRemoval(
		c.Context(), s.storageService, nil, false,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(destroyed, tc.HasLen, 0)
	c.Check(detached, tc.HasLen, 0)
}

// TestClassifyStorageRemovalDetachesPersistent asserts that without destroy
// storage, persistent storage is classified as detached and non persistent
// storage as destroyed.
func (s *storageRemovalSuite) TestClassifyStorageRemovalDetachesPersistent(c *tc.C) {
	defer s.setupMocks(c).Finish()
	unitUUID := tc.Must(c, coreunit.NewUUID)
	persistent := s.newInstance(c, "single-blk/0", true)
	nonPersistent := s.newInstance(c, "single-fs/0", false)

	s.storageService.EXPECT().GetStorageInstancesForUnit(c.Context(), unitUUID).Return(
		[]domainstorage.StorageInstanceInfo{nonPersistent, persistent}, nil,
	)

	destroyed, detached, err := common.ClassifyStorageRemoval(
		c.Context(), s.storageService, []coreunit.UUID{unitUUID}, false,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(destroyed, tc.DeepEquals, []params.Entity{{Tag: "storage-single-fs-0"}})
	c.Check(detached, tc.DeepEquals, []params.Entity{{Tag: "storage-single-blk-0"}})
}

// TestClassifyStorageRemovalDestroyStorage asserts that when destroy storage is
// requested, every attached storage instance is classified as destroyed
// regardless of its persistence.
func (s *storageRemovalSuite) TestClassifyStorageRemovalDestroyStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()
	unitUUID := tc.Must(c, coreunit.NewUUID)
	persistent := s.newInstance(c, "single-blk/0", true)
	nonPersistent := s.newInstance(c, "single-fs/0", false)

	s.storageService.EXPECT().GetStorageInstancesForUnit(c.Context(), unitUUID).Return(
		[]domainstorage.StorageInstanceInfo{persistent, nonPersistent}, nil,
	)

	destroyed, detached, err := common.ClassifyStorageRemoval(
		c.Context(), s.storageService, []coreunit.UUID{unitUUID}, true,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(destroyed, tc.DeepEquals, []params.Entity{
		{Tag: "storage-single-blk-0"},
		{Tag: "storage-single-fs-0"},
	})
	c.Check(detached, tc.HasLen, 0)
}

// TestClassifyStorageRemovalDeduplicatesShared asserts that storage attached to
// more than one of the removed units is only reported once.
func (s *storageRemovalSuite) TestClassifyStorageRemovalDeduplicatesShared(c *tc.C) {
	defer s.setupMocks(c).Finish()
	unitUUID1 := tc.Must(c, coreunit.NewUUID)
	unitUUID2 := tc.Must(c, coreunit.NewUUID)
	shared := s.newInstance(c, "db-dir/0", true)
	nonPersistent := s.newInstance(c, "cache/0", false)

	s.storageService.EXPECT().GetStorageInstancesForUnit(c.Context(), unitUUID1).Return(
		[]domainstorage.StorageInstanceInfo{shared, nonPersistent}, nil,
	)
	s.storageService.EXPECT().GetStorageInstancesForUnit(c.Context(), unitUUID2).Return(
		[]domainstorage.StorageInstanceInfo{shared}, nil,
	)

	destroyed, detached, err := common.ClassifyStorageRemoval(
		c.Context(), s.storageService, []coreunit.UUID{unitUUID1, unitUUID2}, false,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(destroyed, tc.DeepEquals, []params.Entity{{Tag: "storage-cache-0"}})
	c.Check(detached, tc.DeepEquals, []params.Entity{{Tag: "storage-db-dir-0"}})
}

// TestClassifyStorageRemovalError asserts that an error from the storage getter
// is propagated to the caller.
func (s *storageRemovalSuite) TestClassifyStorageRemovalError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	unitUUID := tc.Must(c, coreunit.NewUUID)
	boom := stderrors.New("boom")

	s.storageService.EXPECT().GetStorageInstancesForUnit(c.Context(), unitUUID).Return(
		nil, boom,
	)

	destroyed, detached, err := common.ClassifyStorageRemoval(
		c.Context(), s.storageService, []coreunit.UUID{unitUUID}, false,
	)
	c.Check(err, tc.ErrorIs, boom)
	c.Check(destroyed, tc.IsNil)
	c.Check(detached, tc.IsNil)
}
