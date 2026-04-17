// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// unitStorageAttachSuite is a suite for state tests that focus on retrieving
// attach metadata for storage instances.
type unitStorageAttachSuite struct {
	baseSuite
	storageHelper

	state *State
}

// TestUnitStorageAttachSuite runs all tests in [unitStorageAttachSuite].
func TestUnitStorageAttachSuite(t *testing.T) {
	suite := &unitStorageAttachSuite{}
	suite.storageHelper.dbGetter = &suite.ModelSuite
	tc.Run(t, suite)
}

func (u *unitStorageAttachSuite) SetUpTest(c *tc.C) {
	u.baseSuite.SetUpTest(c)

	u.state = NewState(
		u.TxnRunnerFactory(),
		u.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

// TearDownTest clears suite state and delegates to base teardown so each test
// starts from a clean state.
func (u *unitStorageAttachSuite) TearDownTest(c *tc.C) {
	u.state = nil
	u.baseSuite.TearDownTest(c)
}

// TestGetStorageAttachInfoForStorageInstancesEmpty verifies that calling
// [State.GetStorageAttachInfoForStorageInstances] with no storage UUIDs
// returns an empty result with no error.
func (u *unitStorageAttachSuite) TestGetStorageAttachInfoForStorageInstancesEmpty(c *tc.C) {
	result, err := u.state.GetStorageAttachInfoForStorageInstances(
		c.Context(), nil,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

// TestGetStorageAttachInfoForStorageInstancesNotFound verifies that requesting
// only missing storage instance UUIDs returns
// [domainstorageerrors.StorageInstanceNotFound].
func (u *unitStorageAttachSuite) TestGetStorageAttachInfoForStorageInstancesNotFound(c *tc.C) {
	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	_, err := u.state.GetStorageAttachInfoForStorageInstances(
		c.Context(), []domainstorage.StorageInstanceUUID{storageUUID},
	)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestGetStorageAttachInfoForStorageInstancesNotFoundPartial verifies that
// when any requested storage UUID is missing, the call returns
// [domainstorageerrors.StorageInstanceNotFound].
func (u *unitStorageAttachSuite) TestGetStorageAttachInfoForStorageInstancesNotFoundPartial(c *tc.C) {
	charmUUID := u.newCharmWithStorage(c, "st1", 10)
	existingUUID, _ := u.newModelFilesystemStorageInstance(c, "st1", charmUUID)
	missingUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	_, err := u.state.GetStorageAttachInfoForStorageInstances(
		c.Context(),
		[]domainstorage.StorageInstanceUUID{existingUUID, missingUUID},
	)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestGetStorageAttachInfoForStorageInstancesDeduplicatesRequests verifies
// that duplicate storage UUID inputs are de-duplicated and returned once.
func (u *unitStorageAttachSuite) TestGetStorageAttachInfoForStorageInstancesDeduplicatesRequests(c *tc.C) {
	charmUUID := u.newCharmWithStorage(c, "st1", 10)
	storageUUID, _ := u.newModelFilesystemStorageInstance(c, "st1", charmUUID)

	result, err := u.state.GetStorageAttachInfoForStorageInstances(
		c.Context(),
		[]domainstorage.StorageInstanceUUID{storageUUID, storageUUID, storageUUID},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 1)
	c.Check(result[0].UUID, tc.Equals, storageUUID)
}

// TestGetStorageAttachInfoForStorageInstancesSortedByStorageUUID verifies that
// results are returned in storage UUID order regardless of input order.
func (u *unitStorageAttachSuite) TestGetStorageAttachInfoForStorageInstancesSortedByStorageUUID(c *tc.C) {
	charmUUID := u.newCharmWithStorage(c, "st1", 10)
	storageUUID1, _ := u.newModelFilesystemStorageInstance(c, "st1", charmUUID)
	storageUUID2, _ := u.newModelFilesystemStorageInstance(c, "st1", charmUUID)

	request := []domainstorage.StorageInstanceUUID{storageUUID1, storageUUID2}
	if storageUUID1.String() < storageUUID2.String() {
		request = []domainstorage.StorageInstanceUUID{storageUUID2, storageUUID1}
	}

	result, err := u.state.GetStorageAttachInfoForStorageInstances(
		c.Context(), request,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 2)
	c.Check(result[0].UUID.String() <= result[1].UUID.String(), tc.IsTrue)
}

// TestGetStorageAttachInfoForStorageInstancesFilesystem verifies that
// filesystem-backed storage fields are populated in the returned attach info.
func (u *unitStorageAttachSuite) TestGetStorageAttachInfoForStorageInstancesFilesystem(c *tc.C) {
	charmUUID := u.newCharmWithStorage(c, "st1", 10)
	charmName := u.getCharmMetadataName(c, charmUUID)
	storageUUID, filesystemUUID := u.newModelFilesystemStorageInstance(c, "st1", charmUUID)

	result, err := u.state.GetStorageAttachInfoForStorageInstances(
		c.Context(), []domainstorage.StorageInstanceUUID{storageUUID},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 1)
	c.Check(result[0], tc.DeepEquals, domainstorage.StorageInstanceInfoForAttach{
		StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
			UUID:      storageUUID,
			CharmName: &charmName,
			Filesystem: &domainstorage.StorageInstanceAttachFilesystemInfo{
				UUID:           filesystemUUID,
				ProvisionScope: domainstorage.ProvisionScopeModel,
				SizeMib:        1024,
			},
			Kind:             domainstorage.StorageKindFilesystem,
			Life:             life.Alive,
			RequestedSizeMIB: 1024,
			StorageName:      "st1",
		},
	})
}

// TestGetStorageAttachInfoForStorageInstancesVolume verifies that volume-backed
// storage fields are populated in the returned attach info.
func (u *unitStorageAttachSuite) TestGetStorageAttachInfoForStorageInstancesVolume(c *tc.C) {
	charmUUID := u.newCharmWithStorage(c, "st1", 10)
	charmName := u.getCharmMetadataName(c, charmUUID)
	storageUUID, volumeUUID := u.newModelVolumeStorageInstance(c, "st1", charmUUID)

	result, err := u.state.GetStorageAttachInfoForStorageInstances(
		c.Context(), []domainstorage.StorageInstanceUUID{storageUUID},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 1)
	c.Check(result[0], tc.DeepEquals, domainstorage.StorageInstanceInfoForAttach{
		StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
			UUID:      storageUUID,
			CharmName: &charmName,
			Volume: &domainstorage.StorageInstanceAttachVolumeInfo{
				UUID:           volumeUUID,
				ProvisionScope: domainstorage.ProvisionScopeModel,
				SizeMiB:        2048,
			},
			Kind:             domainstorage.StorageKindBlock,
			Life:             life.Alive,
			RequestedSizeMIB: 2048,
			StorageName:      "st1",
		},
	})
}

// TestGetStorageAttachInfoForStorageInstancesAttachmentPartitioning verifies
// that attachments are partitioned by storage instance and never mixed.
func (u *unitStorageAttachSuite) TestGetStorageAttachInfoForStorageInstancesAttachmentPartitioning(c *tc.C) {
	charmUUID := u.newCharmWithStorage(c, "st1", 10)
	storageUUID1, _ := u.newModelFilesystemStorageInstance(c, "st1", charmUUID)
	storageUUID2, _ := u.newModelFilesystemStorageInstance(c, "st1", charmUUID)

	_, unitUUIDs := u.createIAASApplicationWithNUnits(c, "bar", life.Alive, 3)
	unitUUID1 := unitUUIDs[0]
	unitUUID2 := unitUUIDs[1]
	unitUUID3 := unitUUIDs[2]

	attach1 := u.newStorageInstanceAttachment(c, storageUUID1, unitUUID1)
	attach2 := u.newStorageInstanceAttachment(c, storageUUID1, unitUUID2)
	attach3 := u.newStorageInstanceAttachment(c, storageUUID2, unitUUID3)

	result, err := u.state.GetStorageAttachInfoForStorageInstances(
		c.Context(),
		[]domainstorage.StorageInstanceUUID{storageUUID1, storageUUID2},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 2)

	gotByStorageUUID := make(
		map[domainstorage.StorageInstanceUUID]domainstorage.StorageInstanceInfoForAttach, 2,
	)
	for _, item := range result {
		gotByStorageUUID[item.UUID] = item
	}

	got1, ok1 := gotByStorageUUID[storageUUID1]
	c.Assert(ok1, tc.IsTrue)
	got2, ok2 := gotByStorageUUID[storageUUID2]
	c.Assert(ok2, tc.IsTrue)

	c.Check(got1.StorageInstanceAttachments, tc.SameContents, []domainstorage.StorageInstanceUnitAttachmentID{
		{
			UnitUUID: unitUUID1,
			UUID:     attach1,
		},
		{
			UnitUUID: unitUUID2,
			UUID:     attach2,
		},
	})
	c.Check(got2.StorageInstanceAttachments, tc.SameContents, []domainstorage.StorageInstanceUnitAttachmentID{
		{
			UnitUUID: unitUUID3,
			UUID:     attach3,
		},
	})
}
