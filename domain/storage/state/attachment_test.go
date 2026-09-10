// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	coreunit "github.com/juju/juju/core/unit"
	domainapplicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storage/internal"
)

// attachmentSuite is a test suite for asserting the behaviour of storage
// attachment related methods on [State].
type attachmentSuite struct {
	baseSuite
}

// attachmentUUIDSuite is a test suite for asserting the behaviour of
// [State.GetStorageAttachmentUUIDForStorageInstanceAndUnit].
//
// NOTE (tlm): This was made into its own suite to keep the test name length
// under control.
type attachmentUUIDSuite struct {
	baseSuite
}

// TestAttachmentSuite runs all of the tests contained within [attachmentSuite].
func TestAttachmentSuite(t *testing.T) {
	tc.Run(t, &attachmentSuite{})
}

// TestAttachmentUUIDSuite runs the tests contained in [attachmentUUIDSuite].
func TestAttachmentUUIDSuite(t *testing.T) {
	tc.Run(t, &attachmentUUIDSuite{})
}

// TestUUIDForNotFoundUnit asserts that when a unit does not exist
// [State.GetStorageAttachmentUUIDForStorageInstanceAndUnit] returns a
// [domainapplicationerrors.UnitNotFound] error.
func (s *attachmentUUIDSuite) TestUUIDForNotFoundUnit(c *tc.C) {
	charmUUID := s.newCharm(c)
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	storageInstanceUUID, _ := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "token-store",
	)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), storageInstanceUUID, unitUUID,
	)
	c.Check(err, tc.ErrorIs, domainapplicationerrors.UnitNotFound)
}

// TestUUIDForNotFoundStorageInstance asserts that when a storage
// instance does not exist
// [State.GetStorageAttachmentUUIDForStorageInstanceAndUnit] returns a
// [domainstorageerrors.StorageInstanceNotFound] error.
func (s *attachmentUUIDSuite) TestUUIDForNotFoundStorageInstance(c *tc.C) {
	unitUUID := s.newUnit(c)
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), storageInstanceUUID, unitUUID,
	)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestUUIDForStorageInstanceAndUnit is a happy path test.
func (s *attachmentUUIDSuite) TestUUIDForStorageInstanceAndUnit(c *tc.C) {
	charmUUID := s.newCharm(c)
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	storageInstanceUUID, _ := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "token-store",
	)
	unitUUID := s.newUnit(c)
	storageAttachmentUUID := s.newStorageAttachment(
		c, storageInstanceUUID, unitUUID,
	)

	st := NewState(s.TxnRunnerFactory())
	gotUUID, err := st.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		c.Context(), storageInstanceUUID, unitUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, storageAttachmentUUID)
}

// TestGetStorageInstnaceAttachmentsNotFound asserts that when the storage
// instance does not exist in the model the caller gets back an error satisfying
// [domainstorageerrors.StorageInstanceNotFound].
func (s *attachmentSuite) TestGetStorageInstanceAttachmentsNotFound(c *tc.C) {
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageInstanceAttachments(c.Context(), storageInstanceUUID)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestGetStorageInstanceAttachmentsEmptyResult asserts that when a storage
// instance is not attached to any units an empty slice is returned.
func (s *attachmentSuite) TestGetStorageInstanceAttachmentsEmptyResult(c *tc.C) {
	charmUUID := s.newCharm(c)
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	storageInstanceUUID, _ := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "token-store",
	)

	st := NewState(s.TxnRunnerFactory())
	attachments, err := st.GetStorageInstanceAttachments(c.Context(), storageInstanceUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(attachments, tc.HasLen, 0)
}

// TestGetStorageClassificationForUnits asserts that the minimal storage
// information for units is returned, mapping each unit to its attached
// storage instances with correct volume and filesystem provision scopes.
func (s *attachmentSuite) TestGetStorageClassificationForUnits(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "myapplication")
	unitUUID1, _, _ := s.newUnitForApplication(c, appUUID)
	unitUUID2, _, _ := s.newUnitForApplication(c, appUUID)
	unitUUID3, _, _ := s.newUnitForApplication(c, appUUID)
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)

	modelScope := domainstorage.ProvisionScopeModel
	machineScope := domainstorage.ProvisionScopeMachine

	// 1. Model-scoped volume-less filesystem (e.g. LXD filesystem pool).
	modelFsUUID, modelFsID := s.newFilesystemStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "lxd-fs",
	)
	s.newModelFilesystem(c, modelFsUUID)

	// 2. Machine-scoped volume-less filesystem (e.g. rootfs pool).
	machineFsUUID, machineFsID := s.newFilesystemStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "rootfs-fs",
	)
	s.newMachineFilesystem(c, machineFsUUID)

	// 3. Model-scoped volume (e.g. EBS block storage).
	modelVolUUID, modelVolID := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "ebs-vol",
	)
	s.newModelVolume(c, modelVolUUID)

	// 4. Machine-scoped volume (e.g. loop block storage).
	machineVolUUID, machineVolID := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "loop-vol",
	)
	s.newMachineVolume(c, machineVolUUID)

	// 5. Volume-backed filesystem: machine-scoped filesystem backed by a
	// model-scoped volume (e.g. single-fs=ebs).
	volFsUUID, volFsID := s.newFilesystemStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "ebs-backed-fs",
	)
	s.newMachineFilesystem(c, volFsUUID)
	s.newModelVolume(c, volFsUUID)

	// 6. Orphan storage instance: attached to a unit, but neither a volume
	// nor a filesystem backing record exists.
	orphanUUID, orphanID := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "orphan-store",
	)

	s.newStorageAttachment(c, modelFsUUID, unitUUID1)
	s.newStorageAttachment(c, machineFsUUID, unitUUID1)
	s.newStorageAttachment(c, modelVolUUID, unitUUID1)
	s.newStorageAttachment(c, machineVolUUID, unitUUID2)
	s.newStorageAttachment(c, volFsUUID, unitUUID2)
	s.newStorageAttachment(c, orphanUUID, unitUUID2)

	st := NewState(s.TxnRunnerFactory())
	classifications, err := st.GetStorageClassificationForUnits(
		c.Context(), []string{unitUUID1.String(), unitUUID2.String(), unitUUID3.String()},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(classifications, tc.HasLen, 2)
	c.Check(classifications[unitUUID1.String()], tc.SameContents, []internal.StorageInstanceClassification{
		{
			FilesystemProvisionScope: &modelScope,
			StorageID:                modelFsID,
			StorageUUID:              modelFsUUID.String(),
			VolumeProvisionScope:     nil,
		},
		{
			FilesystemProvisionScope: &machineScope,
			StorageID:                machineFsID,
			StorageUUID:              machineFsUUID.String(),
			VolumeProvisionScope:     nil,
		},
		{
			FilesystemProvisionScope: nil,
			StorageID:                modelVolID,
			StorageUUID:              modelVolUUID.String(),
			VolumeProvisionScope:     &modelScope,
		},
	})
	c.Check(classifications[unitUUID2.String()], tc.SameContents, []internal.StorageInstanceClassification{
		{
			FilesystemProvisionScope: nil,
			StorageID:                machineVolID,
			StorageUUID:              machineVolUUID.String(),
			VolumeProvisionScope:     &machineScope,
		},
		{
			FilesystemProvisionScope: &machineScope,
			StorageID:                volFsID,
			StorageUUID:              volFsUUID.String(),
			VolumeProvisionScope:     &modelScope,
		},
		{
			FilesystemProvisionScope: nil,
			StorageID:                orphanID,
			StorageUUID:              orphanUUID.String(),
			VolumeProvisionScope:     nil,
		},
	})
}

// TestGetStorageClassificationForUnitsEmptyInput asserts that an empty slice
// of unit UUIDs returns an empty map without querying the database.
func (s *attachmentSuite) TestGetStorageClassificationForUnitsEmptyInput(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	classifications, err := st.GetStorageClassificationForUnits(c.Context(), nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(classifications, tc.DeepEquals, map[string][]internal.StorageInstanceClassification{})
}

func (s *attachmentSuite) TestGetStorageInstanceAttachments(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "myapplication")
	unitUUID1, _, _ := s.newUnitForApplication(c, appUUID)
	unitUUID2, _, _ := s.newUnitForApplication(c, appUUID)
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	storageInstanceUUID, _ := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "token-store",
	)
	storageAttachmentUUID1 := s.newStorageAttachment(c, storageInstanceUUID, unitUUID1)
	storageAttachmentUUID2 := s.newStorageAttachment(c, storageInstanceUUID, unitUUID2)

	st := NewState(s.TxnRunnerFactory())
	attachments, err := st.GetStorageInstanceAttachments(c.Context(), storageInstanceUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(
		attachments, tc.SameContents,
		[]domainstorage.StorageAttachmentUUID{
			storageAttachmentUUID2,
			storageAttachmentUUID1,
		},
	)
}
