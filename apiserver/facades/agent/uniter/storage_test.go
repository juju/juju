// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	coreerrors "github.com/juju/juju/core/errors"
	corelife "github.com/juju/juju/core/life"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	storageprovisioningtesting "github.com/juju/juju/domain/storageprovisioning/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type storageSuite struct {
	testing.BaseSuite

	mockBlockDeviceService         *MockBlockDeviceService
	mockApplicationService         *MockApplicationService
	mockStorageProvisioningService *MockStorageProvisioningService
	mockWatcherRegistry            *MockWatcherRegistry
}

func TestStorageSuite(t *stdtesting.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) getAPI(c *tc.C) (*StorageAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.mockBlockDeviceService = NewMockBlockDeviceService(ctrl)
	s.mockApplicationService = NewMockApplicationService(ctrl)
	s.mockStorageProvisioningService = NewMockStorageProvisioningService(ctrl)
	s.mockWatcherRegistry = NewMockWatcherRegistry(ctrl)

	api, err := newStorageAPI(
		s.mockBlockDeviceService,
		s.mockApplicationService,
		s.mockStorageProvisioningService,
		s.mockWatcherRegistry,
		func(ctx context.Context) (common.AuthFunc, error) {
			return func(tag names.Tag) bool {
				return true
			}, nil
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	return api, ctrl
}

func (s *storageSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing the following tests:
- TestWatchUnitStorageAttachments
- TestWatchStorageAttachmentVolume
- TestCAASWatchStorageAttachmentFilesystem
- TestIAASWatchStorageAttachmentFilesystem
- TestDestroyUnitStorageAttachments
- TestWatchStorageAttachmentVolumeAttachmentChanges
- TestWatchStorageAttachmentStorageAttachmentChanges
- TestWatchStorageAttachmentBlockDevicesChange
`)
}

func (s *storageSuite) TestUnitStorageAttachments(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentIDsForUnit(gomock.Any(), unitUUID).Return(
		[]string{"foo/1"}, nil,
	)

	attachments, err := api.UnitStorageAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: unitTag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attachments, tc.DeepEquals, params.StorageAttachmentIdsResults{
		Results: []params.StorageAttachmentIdsResult{
			{
				Result: params.StorageAttachmentIds{
					Ids: []params.StorageAttachmentId{
						{
							StorageTag: "storage-foo-1",
							UnitTag:    unitTag.String(),
						},
					},
				},
			},
		},
	})
}

func (s *storageSuite) TestUnitStorageAttachmentsWithInvalidUnitName(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(
		"", coreunit.InvalidUnitName,
	)

	results, err := api.UnitStorageAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: unitTag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotValid)
}

func (s *storageSuite) TestUnitStorageAttachmentsWithUnitNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(
		"", applicationerrors.UnitNotFound,
	)

	results, err := api.UnitStorageAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: unitTag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestUnitStorageAttachmentsWithInvalidUnitUUID(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentIDsForUnit(gomock.Any(), unitUUID).Return(
		nil, coreerrors.NotValid,
	)

	results, err := api.UnitStorageAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: unitTag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotValid)
}

func (s *storageSuite) TestUnitStorageAttachmentsWithUnitNotFound2(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentIDsForUnit(gomock.Any(), unitUUID).Return(
		nil, applicationerrors.UnitNotFound,
	)

	results, err := api.UnitStorageAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: unitTag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestUnitStorageAttachmentsWithInvalidStorageID(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentIDsForUnit(gomock.Any(), unitUUID).Return(
		[]string{"invalid-storage-id"}, nil,
	)

	results, err := api.UnitStorageAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: unitTag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotValid)
}

func (s *storageSuite) TestStorageAttachmentsForVolume(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)
	saUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)
	vaUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return(saUUID, nil)

	owner := unittesting.GenNewName(c, "foo/0")
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentInfo(
		gomock.Any(), saUUID,
	).Return(storageprovisioning.StorageAttachmentInfo{
		StorageAttachmentUUID: saUUID.String(),
		Owner:                 &owner,
		Kind:                  domainstorage.StorageKindBlock,
		Life:                  domainlife.Alive,
	}, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForStorageID(
		gomock.Any(), "foo/1", unitUUID,
	).Return(vaUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentInfo(
		gomock.Any(), vaUUID,
	).Return(storageprovisioning.VolumeAttachmentInfo{
		HardwareID:      "hwid",
		WWN:             "wwn",
		BlockDeviceName: "sda",
		BlockDeviceLink: "by-id/xyz",
	}, nil)

	results, err := api.StorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result, tc.Equals, params.StorageAttachmentResult{
		Result: params.StorageAttachment{
			StorageTag: "storage-foo-1",
			OwnerTag:   names.NewUnitTag(owner.String()).String(),
			UnitTag:    unitTag.String(),
			Kind:       params.StorageKindBlock,
			Location:   "/dev/disk/by-id/wwn-wwn",
			Life:       corelife.Alive,
		},
	})
}

func (s *storageSuite) TestStorageAttachmentsForVolumeWithNotProvisioned(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)
	saUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)
	vaUUID := storageprovisioningtesting.GenVolumeAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return(saUUID, nil)

	owner := unittesting.GenNewName(c, "foo/0")
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentInfo(
		gomock.Any(), saUUID,
	).Return(storageprovisioning.StorageAttachmentInfo{
		StorageAttachmentUUID: saUUID.String(),
		Owner:                 &owner,
		Kind:                  domainstorage.StorageKindBlock,
		Life:                  domainlife.Alive,
	}, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForStorageID(
		gomock.Any(), "foo/1", unitUUID,
	).Return(vaUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentInfo(
		gomock.Any(), vaUUID,
	).Return(storageprovisioning.VolumeAttachmentInfo{
		HardwareID: "hwid",
		WWN:        "wwn",
	}, nil)

	results, err := api.StorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotProvisioned)
}

func (s *storageSuite) TestStorageAttachmentsForVolumeWithUnitNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(
		"", applicationerrors.UnitNotFound,
	)

	results, err := api.StorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestStorageAttachmentsForVolumeWithStorageInstanceNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return("", storageprovisioningerrors.StorageInstanceNotFound)

	results, err := api.StorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestStorageAttachmentsForVolumeWithStorageAttachmentNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return("", storageprovisioningerrors.StorageAttachmentNotFound)

	results, err := api.StorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestStorageAttachmentsForVolumeWithVolumeAttachmentNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)
	saUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return(saUUID, nil)

	owner := unittesting.GenNewName(c, "foo/0")
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentInfo(
		gomock.Any(), saUUID,
	).Return(storageprovisioning.StorageAttachmentInfo{
		StorageAttachmentUUID: saUUID.String(),
		Owner:                 &owner,
		Kind:                  domainstorage.StorageKindBlock,
		Life:                  domainlife.Alive,
	}, nil)
	s.mockStorageProvisioningService.EXPECT().GetVolumeAttachmentUUIDForStorageID(
		gomock.Any(), "foo/1", unitUUID,
	).Return("", storageprovisioningerrors.VolumeAttachmentNotFound)

	results, err := api.StorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestStorageAttachmentsForFilesystem(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)
	saUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)
	fsaUUID := storageprovisioningtesting.GenFilesystemAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return(saUUID, nil)

	owner := unittesting.GenNewName(c, "foo/0")
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentInfo(
		gomock.Any(), saUUID,
	).Return(storageprovisioning.StorageAttachmentInfo{
		StorageAttachmentUUID: saUUID.String(),
		Owner:                 &owner,
		Kind:                  domainstorage.StorageKindFilesystem,
		Life:                  domainlife.Alive,
	}, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForStorageID(
		gomock.Any(), "foo/1", unitUUID,
	).Return(fsaUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachment(
		gomock.Any(), fsaUUID,
	).Return(storageprovisioning.FilesystemAttachment{
		MountPoint: "/mnt/data",
	}, nil)

	results, err := api.StorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result, tc.Equals, params.StorageAttachmentResult{
		Result: params.StorageAttachment{
			StorageTag: "storage-foo-1",
			OwnerTag:   names.NewUnitTag(owner.String()).String(),
			UnitTag:    unitTag.String(),
			Kind:       params.StorageKindFilesystem,
			Location:   "/mnt/data",
			Life:       corelife.Alive,
		},
	})
}

func (s *storageSuite) TestStorageAttachmentsForFilesystemWithStorageInstanceNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)
	saUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return(saUUID, nil)

	owner := unittesting.GenNewName(c, "foo/0")
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentInfo(
		gomock.Any(), saUUID,
	).Return(storageprovisioning.StorageAttachmentInfo{
		StorageAttachmentUUID: saUUID.String(),
		Owner:                 &owner,
		Kind:                  domainstorage.StorageKindFilesystem,
		Life:                  domainlife.Alive,
	}, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForStorageID(
		gomock.Any(), "foo/1", unitUUID,
	).Return("", storageprovisioningerrors.StorageInstanceNotFound)

	results, err := api.StorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestStorageAttachmentsForFilesystemWithFilesystemAttachmentNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)
	saUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return(saUUID, nil)

	owner := unittesting.GenNewName(c, "foo/0")
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentInfo(
		gomock.Any(), saUUID,
	).Return(storageprovisioning.StorageAttachmentInfo{
		StorageAttachmentUUID: saUUID.String(),
		Owner:                 &owner,
		Kind:                  domainstorage.StorageKindFilesystem,
		Life:                  domainlife.Alive,
	}, nil)
	s.mockStorageProvisioningService.EXPECT().GetFilesystemAttachmentUUIDForStorageID(
		gomock.Any(), "foo/1", unitUUID,
	).Return("", storageprovisioningerrors.FilesystemAttachmentNotFound)

	results, err := api.StorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestStorageAttachmentLife(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), unitUUID, "foo/1",
	).Return(domainlife.Alive, nil)

	results, err := api.StorageAttachmentLife(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: corelife.Alive,
			},
		},
	})
}

func (s *storageSuite) TestStorageAttachmentLifeWithInvalidUnitName(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(
		"", coreunit.InvalidUnitName,
	)

	results, err := api.StorageAttachmentLife(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotValid)
}

func (s *storageSuite) TestStorageAttachmentLifeWithUnitNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(
		"", applicationerrors.UnitNotFound,
	)

	results, err := api.StorageAttachmentLife(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestStorageAttachmentLifeWithInvalidUnitUUID(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), unitUUID, "foo/1",
	).Return(-1, coreerrors.NotValid)

	results, err := api.StorageAttachmentLife(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotValid)
}

func (s *storageSuite) TestStorageAttachmentLifeWithUnitNotFound2(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), unitUUID, "foo/1",
	).Return(-1, applicationerrors.UnitNotFound)

	results, err := api.StorageAttachmentLife(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestStorageAttachmentLifeWithStorageInstanceNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), unitUUID, "foo/1",
	).Return(-1, storageprovisioningerrors.StorageInstanceNotFound)

	results, err := api.StorageAttachmentLife(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestStorageAttachmentLifeWithAttachmentNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), unitUUID, "foo/1",
	).Return(-1, storageprovisioningerrors.StorageAttachmentNotFound)

	results, err := api.StorageAttachmentLife(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestWatchUnitStorageAttachments(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	changed := make(chan []string, 1)
	changed <- []string{"foo/1", "bar/1"}
	sourceWatcher := watchertest.NewMockStringsWatcher(changed)

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().WatchStorageAttachmentsForUnit(gomock.Any(), unitUUID).Return(sourceWatcher, nil)
	s.mockWatcherRegistry.EXPECT().Register(gomock.Any(), sourceWatcher).Return("66", nil)

	results, err := api.WatchUnitStorageAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.StringsWatcherId, tc.Equals, "66")
	c.Assert(result.Changes, tc.DeepEquals, []string{"foo/1", "bar/1"})
}

func (s *storageSuite) TestWatchUnitStorageAttachmentsWithUnitNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return("", applicationerrors.UnitNotFound)

	results, err := api.WatchUnitStorageAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestWatchStorageAttachments(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	changed := make(chan struct{}, 1)
	changed <- struct{}{}
	sourceWatcher := watchertest.NewMockNotifyWatcher(changed)

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)
	storageAttachmentUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return(storageAttachmentUUID, nil)
	s.mockStorageProvisioningService.EXPECT().WatchStorageAttachment(
		gomock.Any(), storageAttachmentUUID,
	).Return(sourceWatcher, nil)
	s.mockWatcherRegistry.EXPECT().Register(gomock.Any(), sourceWatcher).Return("66", nil)

	results, err := api.WatchStorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				UnitTag:    unitTag.String(),
				StorageTag: "storage-foo-1",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.NotifyWatcherId, tc.Equals, "66")
}

func (s *storageSuite) TestWatchStorageAttachmentsWithInvalidUnitUUID(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return("", coreerrors.NotValid)

	results, err := api.WatchStorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				UnitTag:    unitTag.String(),
				StorageTag: "storage-foo-1",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotValid)
}

func (s *storageSuite) TestWatchStorageAttachmentsWithUnitNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return("", applicationerrors.UnitNotFound)

	results, err := api.WatchStorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				UnitTag:    unitTag.String(),
				StorageTag: "storage-foo-1",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestWatchStorageAttachmentsWithStorageInstanceNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return("", storageprovisioningerrors.StorageInstanceNotFound)

	results, err := api.WatchStorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				UnitTag:    unitTag.String(),
				StorageTag: "storage-foo-1",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *storageSuite) TestWatchStorageAttachmentsWithStorageAttachmentNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return("", storageprovisioningerrors.StorageAttachmentNotFound)

	results, err := api.WatchStorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				UnitTag:    unitTag.String(),
				StorageTag: "storage-foo-1",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error.Code, tc.Equals, params.CodeNotFound)
}
