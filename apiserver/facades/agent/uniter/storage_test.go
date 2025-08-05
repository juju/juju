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
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
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
	c.Assert(err, tc.IsNil)
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
	c.Assert(err, tc.IsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageIDsForUnit(gomock.Any(), unitUUID).Return(
		[]corestorage.ID{
			corestorage.ID("foo/1"),
		}, nil,
	)

	attachments, err := api.UnitStorageAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: unitTag.String()},
		},
	})
	c.Assert(err, tc.IsNil)
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
	c.Assert(err, tc.IsNil)

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
	c.Assert(err, tc.IsNil)

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
	c.Assert(err, tc.IsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageIDsForUnit(gomock.Any(), unitUUID).Return(
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
	c.Assert(err, tc.IsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageIDsForUnit(gomock.Any(), unitUUID).Return(
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
	c.Assert(err, tc.IsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.mockStorageProvisioningService.EXPECT().GetStorageIDsForUnit(gomock.Any(), unitUUID).Return(
		nil, corestorage.InvalidStorageID,
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

func (s *storageSuite) TestStorageAttachmentLife(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.IsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	storageID := corestorage.ID("foo/1")
	s.mockStorageProvisioningService.EXPECT().GetAttachmentLife(
		gomock.Any(), unitUUID, storageID,
	).Return(domainlife.Alive, nil)

	results, err := api.StorageAttachmentLife(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-foo-1",
				UnitTag:    unitTag.String(),
			},
		},
	})
	c.Assert(err, tc.IsNil)
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
	c.Assert(err, tc.IsNil)

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
	c.Assert(err, tc.IsNil)

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
	c.Assert(err, tc.IsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	storageID := corestorage.ID("foo/1")
	s.mockStorageProvisioningService.EXPECT().GetAttachmentLife(
		gomock.Any(), unitUUID, storageID,
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

func (s *storageSuite) TestStorageAttachmentLifeWithInvalidStorageID(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.IsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	storageID := corestorage.ID("foo/1")
	s.mockStorageProvisioningService.EXPECT().GetAttachmentLife(
		gomock.Any(), unitUUID, storageID,
	).Return(-1, corestorage.InvalidStorageID)

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
	c.Assert(err, tc.IsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	storageID := corestorage.ID("foo/1")
	s.mockStorageProvisioningService.EXPECT().GetAttachmentLife(
		gomock.Any(), unitUUID, storageID,
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

func (s *storageSuite) TestStorageAttachmentLifeWithAttachmentNotFound(c *tc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("wordpress/0")
	unitName, err := coreunit.NewName(unitTag.Id())
	c.Assert(err, tc.IsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.mockApplicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	storageID := corestorage.ID("foo/1")
	s.mockStorageProvisioningService.EXPECT().GetAttachmentLife(
		gomock.Any(), unitUUID, storageID,
	).Return(-1, storageprovisioningerrors.AttachmentNotFound)

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
