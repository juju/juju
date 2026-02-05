// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"
	"time"

	"github.com/juju/tc"

	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	"github.com/juju/juju/domain/storageprovisioning"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/uuid"
)

// storageSuite is a test suite for asserting the behaviour of general
// methods on [State].
type storageSuite struct {
	baseSuite
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestGetStorageResourceTagInfoForModel(c *tc.C) {
	controllerUUID := uuid.MustNewUUID().String()

	_, err := s.DB().ExecContext(
		c.Context(),
		"INSERT INTO model_config (key, value) VALUES (?, ?)",
		"resource_tags",
		"a=x b=y",
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "", "", "", "", "")
`,
		s.ModelUUID(),
		controllerUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	resourceTags, err := st.GetStorageResourceTagInfoForModel(
		c.Context(), "resource_tags",
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(resourceTags, tc.DeepEquals, storageprovisioning.ModelResourceTagInfo{
		BaseResourceTags: "a=x b=y",
		ModelUUID:        s.ModelUUID(),
		ControllerUUID:   controllerUUID,
	})
}

func (s *storageSuite) TestCreateStorageInstanceWithExistingFilesystem(c *tc.C) {
	poolUUID := s.newStoragePool(c, "mypool", "myprovider", nil)

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	now := time.Now().UTC()

	args := domainstorageinternal.CreateStorageInstanceWithExistingFilesystem{
		UUID:                      storageInstanceUUID,
		Name:                      domainstorage.Name("data"),
		Kind:                      domainstorage.StorageKindFilesystem,
		StoragePoolUUID:           poolUUID,
		RequestedSizeMiB:          1024,
		FilesystemUUID:            filesystemUUID,
		FilesystemProvisionScope:  domainstorageprov.ProvisionScopeModel,
		FilesystemSize:            2048,
		FilesystemProviderID:      "fs-12345",
		FilesystemStatusID:        1,
		FilesystemStatusMessage:   "fs-ready",
		FilesystemStatusUpdatedAt: now,
	}

	st := NewState(s.TxnRunnerFactory())
	storageID, err := st.CreateStorageInstanceWithExistingFilesystem(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(storageID, tc.Matches, "data/[0-9]+")

	// Verify storage instance was created
	var (
		gotStorageUUID   string
		gotStorageName   string
		gotStorageKindID int
		gotRequestedSize uint64
		gotPoolUUID      string
	)
	err = s.DB().QueryRow(`
SELECT uuid, storage_name, storage_kind_id, requested_size_mib, storage_pool_uuid
FROM storage_instance
WHERE uuid = ?
`, storageInstanceUUID.String()).Scan(
		&gotStorageUUID,
		&gotStorageName,
		&gotStorageKindID,
		&gotRequestedSize,
		&gotPoolUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotStorageUUID, tc.Equals, storageInstanceUUID.String())
	c.Check(gotStorageName, tc.Equals, "data")
	c.Check(gotStorageKindID, tc.Equals, int(domainstorage.StorageKindFilesystem))
	c.Check(gotRequestedSize, tc.Equals, uint64(1024))
	c.Check(gotPoolUUID, tc.Equals, poolUUID.String())

	// Verify filesystem was created
	var (
		gotFilesystemUUID       string
		gotProvisionScopeID     int
		gotFilesystemProviderID string
		gotFilesystemSize       uint64
	)
	err = s.DB().QueryRow(`
SELECT uuid, provision_scope_id, provider_id, size_mib
FROM storage_filesystem
WHERE uuid = ?
`, filesystemUUID.String()).Scan(
		&gotFilesystemUUID,
		&gotProvisionScopeID,
		&gotFilesystemProviderID,
		&gotFilesystemSize,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotFilesystemUUID, tc.Equals, filesystemUUID.String())
	c.Check(gotProvisionScopeID, tc.Equals, int(domainstorageprov.ProvisionScopeModel))
	c.Check(gotFilesystemProviderID, tc.Equals, "fs-12345")
	c.Check(gotFilesystemSize, tc.Equals, uint64(2048))

	// Verify link between storage instance and filesystem
	var (
		gotLinkStorageUUID    string
		gotLinkFilesystemUUID string
	)
	err = s.DB().QueryRow(`
SELECT storage_instance_uuid, storage_filesystem_uuid
FROM storage_instance_filesystem
WHERE storage_instance_uuid = ?
`, storageInstanceUUID.String()).Scan(
		&gotLinkStorageUUID,
		&gotLinkFilesystemUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotLinkStorageUUID, tc.Equals, storageInstanceUUID.String())
	c.Check(gotLinkFilesystemUUID, tc.Equals, filesystemUUID.String())

	// Verify filesystem status was created
	var (
		gotFilesystemStatusID      int
		gotFilesystemStatusMessage string
		gotFilesystemStatusUpdated time.Time
	)
	err = s.DB().QueryRow(`
SELECT status_id, message, updated_at
FROM storage_filesystem_status
WHERE filesystem_uuid = ?
`, filesystemUUID.String()).Scan(
		&gotFilesystemStatusID,
		&gotFilesystemStatusMessage,
		&gotFilesystemStatusUpdated,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotFilesystemStatusID, tc.Equals, 1)
	c.Check(gotFilesystemStatusMessage, tc.Equals, "fs-ready")
	c.Check(gotFilesystemStatusUpdated.Equal(now), tc.Equals, true)
}

// TestCreateStorageInstanceWithExistingFilesystemPoolNotFound asserts that
// when the storage pool does not exist, a StoragePoolNotFound error is
// returned.
func (s *storageSuite) TestCreateStorageInstanceWithExistingFilesystemPoolNotFound(c *tc.C) {
	nonExistentPoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)

	args := domainstorageinternal.CreateStorageInstanceWithExistingFilesystem{
		UUID:                     storageInstanceUUID,
		Name:                     domainstorage.Name("data"),
		Kind:                     domainstorage.StorageKindFilesystem,
		StoragePoolUUID:          nonExistentPoolUUID,
		RequestedSizeMiB:         1024,
		FilesystemUUID:           filesystemUUID,
		FilesystemProvisionScope: domainstorageprov.ProvisionScopeModel,
		FilesystemSize:           2048,
		FilesystemProviderID:     "fs-12345",
	}

	st := NewState(s.TxnRunnerFactory())
	_, err := st.CreateStorageInstanceWithExistingFilesystem(c.Context(), args)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *storageSuite) TestCreateStorageInstanceWithExistingVolumeBackedFilesystem(c *tc.C) {
	poolUUID := s.newStoragePool(c, "mypool", "myprovider", nil)

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	volumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	now := time.Now().UTC()

	fsArgs := domainstorageinternal.CreateStorageInstanceWithExistingFilesystem{
		UUID:                      storageInstanceUUID,
		Name:                      domainstorage.Name("disk"),
		Kind:                      domainstorage.StorageKindBlock,
		StoragePoolUUID:           poolUUID,
		RequestedSizeMiB:          2048,
		FilesystemUUID:            filesystemUUID,
		FilesystemProvisionScope:  domainstorageprov.ProvisionScopeMachine,
		FilesystemSize:            4096,
		FilesystemProviderID:      "fs-abc123",
		FilesystemStatusID:        1,
		FilesystemStatusMessage:   "fs-ready",
		FilesystemStatusUpdatedAt: now,
	}
	args := domainstorageinternal.CreateStorageInstanceWithExistingVolumeBackedFilesystem{
		CreateStorageInstanceWithExistingFilesystem: fsArgs,

		VolumeUUID:            volumeUUID,
		VolumeProvisionScope:  domainstorageprov.ProvisionScopeModel,
		VolumeSize:            4096,
		VolumeProviderID:      "vol-xyz789",
		VolumeHardwareID:      "hw-001",
		VolumeWWN:             "wwn-002",
		VolumePersistent:      true,
		VolumeStatusID:        2,
		VolumeStatusMessage:   "vol-ready",
		VolumeStatusUpdatedAt: now,
	}

	st := NewState(s.TxnRunnerFactory())
	storageID, err := st.CreateStorageInstanceWithExistingVolumeBackedFilesystem(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(storageID, tc.Matches, "disk/[0-9]+")

	// Verify storage instance was created
	var (
		gotStorageUUID   string
		gotStorageName   string
		gotStorageKindID int
		gotRequestedSize uint64
		gotPoolUUID      string
	)
	err = s.DB().QueryRow(`
SELECT uuid, storage_name, storage_kind_id, requested_size_mib, storage_pool_uuid
FROM storage_instance
WHERE uuid = ?
`, storageInstanceUUID.String()).Scan(
		&gotStorageUUID,
		&gotStorageName,
		&gotStorageKindID,
		&gotRequestedSize,
		&gotPoolUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotStorageUUID, tc.Equals, storageInstanceUUID.String())
	c.Check(gotStorageName, tc.Equals, "disk")
	c.Check(gotStorageKindID, tc.Equals, int(domainstorage.StorageKindBlock))
	c.Check(gotRequestedSize, tc.Equals, uint64(2048))
	c.Check(gotPoolUUID, tc.Equals, poolUUID.String())

	// Verify filesystem was created
	var (
		gotFilesystemUUID       string
		gotProvisionScopeID     int
		gotFilesystemProviderID string
		gotFilesystemSize       uint64
	)
	err = s.DB().QueryRow(`
SELECT uuid, provision_scope_id, provider_id, size_mib
FROM storage_filesystem
WHERE uuid = ?
`, filesystemUUID.String()).Scan(
		&gotFilesystemUUID,
		&gotProvisionScopeID,
		&gotFilesystemProviderID,
		&gotFilesystemSize,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotFilesystemUUID, tc.Equals, filesystemUUID.String())
	c.Check(gotProvisionScopeID, tc.Equals, int(domainstorageprov.ProvisionScopeMachine))
	c.Check(gotFilesystemProviderID, tc.Equals, "fs-abc123")
	c.Check(gotFilesystemSize, tc.Equals, uint64(4096))

	// Verify volume was created
	var (
		gotVolumeUUID          string
		gotVolProvisionScopeID int
		gotVolumeProviderID    string
		gotVolumeSize          uint64
		gotVolumeHardwareID    string
		gotVolumeWWN           string
		gotVolumePersistent    bool
	)
	err = s.DB().QueryRow(`
SELECT uuid, provision_scope_id, provider_id, size_mib, hardware_id, wwn, persistent
FROM storage_volume
WHERE uuid = ?
`, volumeUUID.String()).Scan(
		&gotVolumeUUID,
		&gotVolProvisionScopeID,
		&gotVolumeProviderID,
		&gotVolumeSize,
		&gotVolumeHardwareID,
		&gotVolumeWWN,
		&gotVolumePersistent,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotVolumeUUID, tc.Equals, volumeUUID.String())
	c.Check(gotVolProvisionScopeID, tc.Equals, int(domainstorageprov.ProvisionScopeModel))
	c.Check(gotVolumeProviderID, tc.Equals, "vol-xyz789")
	c.Check(gotVolumeSize, tc.Equals, uint64(4096))
	c.Check(gotVolumeHardwareID, tc.Equals, "hw-001")
	c.Check(gotVolumeWWN, tc.Equals, "wwn-002")
	c.Check(gotVolumePersistent, tc.Equals, true)

	// Verify link between storage instance and filesystem
	var (
		gotLinkStorageUUID    string
		gotLinkFilesystemUUID string
	)
	err = s.DB().QueryRow(`
SELECT storage_instance_uuid, storage_filesystem_uuid
FROM storage_instance_filesystem
WHERE storage_instance_uuid = ?
`, storageInstanceUUID.String()).Scan(
		&gotLinkStorageUUID,
		&gotLinkFilesystemUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotLinkStorageUUID, tc.Equals, storageInstanceUUID.String())
	c.Check(gotLinkFilesystemUUID, tc.Equals, filesystemUUID.String())

	// Verify link between storage instance and volume
	var (
		gotLinkStorageUUID2 string
		gotLinkVolumeUUID   string
	)
	err = s.DB().QueryRow(`
SELECT storage_instance_uuid, storage_volume_uuid
FROM storage_instance_volume
WHERE storage_instance_uuid = ?
`, storageInstanceUUID.String()).Scan(
		&gotLinkStorageUUID2,
		&gotLinkVolumeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotLinkStorageUUID2, tc.Equals, storageInstanceUUID.String())
	c.Check(gotLinkVolumeUUID, tc.Equals, volumeUUID.String())

	// Verify filesystem status was created
	var (
		gotFilesystemStatusID      int
		gotFilesystemStatusMessage string
		gotFilesystemStatusUpdated time.Time
	)
	err = s.DB().QueryRow(`
SELECT status_id, message, updated_at
FROM storage_filesystem_status
WHERE filesystem_uuid = ?
`, filesystemUUID.String()).Scan(
		&gotFilesystemStatusID,
		&gotFilesystemStatusMessage,
		&gotFilesystemStatusUpdated,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotFilesystemStatusID, tc.Equals, 1)
	c.Check(gotFilesystemStatusMessage, tc.Equals, "fs-ready")
	c.Check(gotFilesystemStatusUpdated.Equal(now), tc.Equals, true)

	// Verify volume status was created
	var (
		gotVolumeStatusID      int
		gotVolumeStatusMessage string
		gotVolumeStatusUpdated time.Time
	)
	err = s.DB().QueryRow(`
SELECT status_id, message, updated_at
FROM storage_volume_status
WHERE volume_uuid = ?
`, volumeUUID.String()).Scan(
		&gotVolumeStatusID,
		&gotVolumeStatusMessage,
		&gotVolumeStatusUpdated,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotVolumeStatusID, tc.Equals, 2)
	c.Check(gotVolumeStatusMessage, tc.Equals, "vol-ready")
	c.Check(gotVolumeStatusUpdated.Equal(now), tc.Equals, true)
}

// TestCreateStorageInstanceWithExistingVolumeBackedFilesystemPoolNotFound
// asserts that when the storage pool does not exist, a StoragePoolNotFound
// error is returned.
func (s *storageSuite) TestCreateStorageInstanceWithExistingVolumeBackedFilesystemPoolNotFound(c *tc.C) {
	nonExistentPoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	volumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)

	fsArgs := domainstorageinternal.CreateStorageInstanceWithExistingFilesystem{
		UUID:                     storageInstanceUUID,
		Name:                     domainstorage.Name("disk"),
		Kind:                     domainstorage.StorageKindBlock,
		StoragePoolUUID:          nonExistentPoolUUID,
		RequestedSizeMiB:         2048,
		FilesystemUUID:           filesystemUUID,
		FilesystemProvisionScope: domainstorageprov.ProvisionScopeModel,
		FilesystemSize:           4096,
		FilesystemProviderID:     "fs-abc123",
	}
	args := domainstorageinternal.CreateStorageInstanceWithExistingVolumeBackedFilesystem{
		CreateStorageInstanceWithExistingFilesystem: fsArgs,

		VolumeUUID:           volumeUUID,
		VolumeProvisionScope: domainstorageprov.ProvisionScopeMachine,
		VolumeSize:           4096,
		VolumeProviderID:     "vol-xyz789",
		VolumeHardwareID:     "hw-001",
		VolumeWWN:            "wwn-002",
		VolumePersistent:     false,
	}

	st := NewState(s.TxnRunnerFactory())
	_, err := st.CreateStorageInstanceWithExistingVolumeBackedFilesystem(c.Context(), args)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}
