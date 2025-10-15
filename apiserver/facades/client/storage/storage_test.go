// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/rpc/params"
)

type storageSuite struct {
	baseStorageSuite
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenerios:
- ListStorageDetails but retrieving units returns an error (is this a useful test?)
- ListStorageDetails but retrieving the unit's storage attachements returns an error (is this a useful test?)
- TestStorageListEmpty
- TestStorageListFilesystem
- TestStorageListVolume
- TestStorageListError
- TestStorageListInstanceError
- TestStorageListFilesystemError
- TestShowStorageEmpty
- TestShowStorageInvalidTag
- TestShowStorage
- TestShowStorageInvalidId
- TestRemove
- TestDetach
- TestDetachSpecifiedNotFound
- TestDetachAttachmentNotFoundConcurrent
- TestDetachNoAttachmentsStorageNotFound
- TestAttach
- TestImportFilesystem
- TestImportFilesystemVolumeBacked
- TestImportFilesystemError
- TestImportFilesystemNotSupported
- TestImportFilesystemK8sProvider
- TestImportFilesystemVolumeBackedNotSupported
- TestImportValidationErrors
- TestListStorageAsAdminOnNotOwnedModel
- TestListStorageAsNonAdminOnNotOwnedModel
`)
}

func (s *storageSuite) TestListStorageDetails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	u0n := unit.Name("mysql/0")
	m0 := machine.Name("0")
	u1n := unit.Name("mysql/1")
	sInstanceUUID0 := storagetesting.GenStorageInstanceUUID(c)
	sInstanceUUID1 := storagetesting.GenStorageInstanceUUID(c)
	s.storageService.EXPECT().GetAllStorageInstances(gomock.Any()).
		Return([]domainstorage.StorageInstanceDetails{
			{
				UUID:       sInstanceUUID0,
				ID:         "pgdata/0",
				Owner:      &u0n,
				Kind:       domainstorage.StorageKindBlock,
				Life:       life.Alive,
				Persistent: true,
			},
			{
				UUID:       sInstanceUUID1,
				ID:         "data/1",
				Owner:      &u1n,
				Kind:       domainstorage.StorageKindFilesystem,
				Life:       life.Alive,
				Persistent: false,
			},
		}, nil)
	s.storageService.EXPECT().GetVolumeWithAttachments(gomock.Any(), sInstanceUUID0).
		Return(map[string]domainstorage.VolumeDetails{
			"pgdata/0": {
				StorageID: "pgdata/0",
				Status: corestatus.StatusInfo{
					Status:  corestatus.Attaching,
					Message: "attaching the volumez",
				},
				Attachments: []domainstorage.VolumeAttachmentDetails{
					{
						AttachmentDetails: domainstorage.AttachmentDetails{
							Life:    life.Alive,
							Unit:    u0n,
							Machine: &m0,
						},
						BlockDeviceUUID: "block-device-uuid-0",
					},
				},
			},
		}, nil)
	s.blockDeviceService.EXPECT().GetBlockDevices(
		gomock.Any(),
		domainblockdevice.BlockDeviceUUID("block-device-uuid-0"),
	).
		Return([]domainblockdevice.BlockDeviceDetails{
			{
				UUID: "block-device-uuid-0",
				BlockDevice: coreblockdevice.BlockDevice{
					HardwareId:  "hwid-0",
					WWN:         "wwn",
					DeviceName:  "abc",
					DeviceLinks: []string{"xyz"},
				},
			},
		}, nil)
	s.storageService.EXPECT().GetFilesystemWithAttachments(gomock.Any(), sInstanceUUID1).
		Return(map[string]domainstorage.FilesystemDetails{
			"data/1": {
				StorageID: "data/1",
				Status: corestatus.StatusInfo{
					Status:  corestatus.Attached,
					Message: "all good",
				},
				Attachments: []domainstorage.FilesystemAttachmentDetails{
					{
						AttachmentDetails: domainstorage.AttachmentDetails{
							Life: life.Alive,
							Unit: u1n,
						},
						MountPoint: "/data",
					},
				},
			},
		}, nil)

	result, err := s.api.ListStorageDetails(c.Context(), params.StorageFilters{Filters: []params.StorageFilter{{}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StorageDetailsListResults{
		Results: []params.StorageDetailsListResult{
			{
				Result: []params.StorageDetails{
					{
						StorageTag: "storage-pgdata-0",
						OwnerTag:   "unit-mysql-0",
						Kind:       1,
						Status: params.EntityStatus{
							Status: corestatus.Attaching,
							Info:   "attaching the volumez",
						},
						Life:       "alive",
						Persistent: true,
						Attachments: map[string]params.StorageAttachmentDetails{
							"unit-mysql-0": {
								StorageTag: "storage-pgdata-0",
								UnitTag:    "unit-mysql-0",
								MachineTag: "machine-0",
								Life:       "alive",
								Location:   "/dev/disk/by-id/wwn-wwn",
							},
						},
					},
					{
						StorageTag: "storage-data-1",
						OwnerTag:   "unit-mysql-1",
						Kind:       2,
						Status: params.EntityStatus{
							Status: corestatus.Attached,
							Info:   "all good",
						},
						Life:       "alive",
						Persistent: false,
						Attachments: map[string]params.StorageAttachmentDetails{
							"unit-mysql-1": {
								StorageTag: "storage-data-1",
								UnitTag:    "unit-mysql-1",
								Life:       "alive",
								Location:   "/data",
							},
						},
					},
				},
			},
		},
	})
}
