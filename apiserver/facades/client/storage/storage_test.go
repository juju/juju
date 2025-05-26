// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	facadestorage "github.com/juju/juju/apiserver/facades/client/storage"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
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
- ListStorageDetails but retrieving the unit's storage attachements returns an error (is this a useful test?)`)
}

func (s *storageSuite) TestStorageListEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageAccessor.allStorageInstances = func() ([]state.StorageInstance, error) {
		s.stub.AddCall(allStorageInstancesCall)
		return []state.StorageInstance{}, nil
	}

	found, err := s.api.ListStorageDetails(
		c.Context(),
		params.StorageFilters{Filters: []params.StorageFilter{{}}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Error, tc.IsNil)
	c.Assert(found.Results[0].Result, tc.HasLen, 0)
	s.assertCalls(c, []string{allStorageInstancesCall})
}

func (s *storageSuite) TestStorageListFilesystem(c *tc.C) {
	defer s.setupMocks(c).Finish()

	found, err := s.api.ListStorageDetails(
		c.Context(),
		params.StorageFilters{Filters: []params.StorageFilter{{}}},
	)
	c.Assert(err, tc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceFilesystemCall,
		storageInstanceAttachmentsCall,
		storageInstanceCall,
		storageInstanceFilesystemCall,
		storageInstanceFilesystemAttachmentCall,
	}
	s.assertCalls(c, expectedCalls)

	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Error, tc.IsNil)
	c.Assert(found.Results[0].Result, tc.HasLen, 1)
	wantedDetails := s.createTestStorageDetails()
	c.Assert(found.Results[0].Result[0], tc.DeepEquals, wantedDetails)
}

func (s *storageSuite) TestStorageListVolume(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageInstance.kind = state.StorageKindBlock
	found, err := s.api.ListStorageDetails(
		c.Context(),
		params.StorageFilters{Filters: []params.StorageFilter{{}}},
	)
	c.Assert(err, tc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceVolumeCall,
		storageInstanceAttachmentsCall,
		storageInstanceCall,
		storageInstanceVolumeCall,
	}
	s.assertCalls(c, expectedCalls)

	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Error, tc.IsNil)
	c.Assert(found.Results[0].Result, tc.HasLen, 1)
	wantedDetails := s.createTestStorageDetails()
	wantedDetails.Kind = params.StorageKindBlock
	wantedDetails.Status.Status = status.Attached
	c.Assert(found.Results[0].Result[0], tc.DeepEquals, wantedDetails)
}

func (s *storageSuite) TestStorageListError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	msg := "list test error"
	s.storageAccessor.allStorageInstances = func() ([]state.StorageInstance, error) {
		s.stub.AddCall(allStorageInstancesCall)
		return []state.StorageInstance{}, errors.New(msg)
	}

	found, err := s.api.ListStorageDetails(
		c.Context(),
		params.StorageFilters{Filters: []params.StorageFilter{{}}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Error, tc.ErrorMatches, msg)

	expectedCalls := []string{allStorageInstancesCall}
	s.assertCalls(c, expectedCalls)
}

func (s *storageSuite) TestStorageListInstanceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	msg := "list test error"
	s.storageAccessor.storageInstance = func(sTag names.StorageTag) (state.StorageInstance, error) {
		s.stub.AddCall(storageInstanceCall)
		c.Assert(sTag, tc.DeepEquals, s.storageTag)
		return nil, errors.New(msg)
	}

	found, err := s.api.ListStorageDetails(
		c.Context(),
		params.StorageFilters{Filters: []params.StorageFilter{{}}},
	)
	c.Assert(err, tc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceFilesystemCall,
		storageInstanceAttachmentsCall,
		storageInstanceCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Error, tc.ErrorMatches,
		fmt.Sprintf("getting details for storage data/0: getting storage instance: %v", msg),
	)
}

func (s *storageSuite) TestStorageListFilesystemError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	msg := "list test error"
	s.storageAccessor.storageInstanceFilesystem = func(sTag names.StorageTag) (state.Filesystem, error) {
		s.stub.AddCall(storageInstanceFilesystemCall)
		c.Assert(sTag, tc.DeepEquals, s.storageTag)
		return nil, errors.New(msg)
	}

	found, err := s.api.ListStorageDetails(
		c.Context(),
		params.StorageFilters{Filters: []params.StorageFilter{{}}},
	)
	c.Assert(err, tc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceFilesystemCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Error, tc.ErrorMatches,
		fmt.Sprintf("getting details for storage data/0: %v", msg),
	)
}

func (s *storageSuite) createTestStorageDetails() params.StorageDetails {
	return params.StorageDetails{
		StorageTag: s.storageTag.String(),
		OwnerTag:   s.unitTag.String(),
		Kind:       params.StorageKindFilesystem,
		Life:       "dying",
		Status: params.EntityStatus{
			Status: "attached",
		},
		Attachments: map[string]params.StorageAttachmentDetails{
			s.unitTag.String(): {
				StorageTag: s.storageTag.String(),
				UnitTag:    s.unitTag.String(),
				MachineTag: s.machineTag.String(),
				Location:   "", // location
				Life:       "alive",
			},
		},
	}
}

func (s *storageSuite) assertInstanceInfoError(c *tc.C, obtained params.StorageDetailsResult, wanted params.StorageDetailsResult, expected string) {
	if expected != "" {
		c.Assert(errors.Cause(obtained.Error), tc.ErrorMatches, fmt.Sprintf(".*%v.*", expected))
		c.Assert(obtained.Result, tc.IsNil)
	} else {
		c.Assert(obtained.Error, tc.IsNil)
		c.Assert(obtained, tc.DeepEquals, wanted)
	}
}

func (s *storageSuite) TestShowStorageEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	found, err := s.api.StorageDetails(c.Context(), params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 0)
}

func (s *storageSuite) TestShowStorageInvalidTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Only storage tags are permitted
	found, err := s.api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Error, tc.ErrorMatches, `"machine-1" is not a valid storage tag`)
}

func (s *storageSuite) TestShowStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	entity := params.Entity{Tag: s.storageTag.String()}

	found, err := s.api.StorageDetails(
		c.Context(),
		params.Entities{Entities: []params.Entity{entity}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 1)

	one := found.Results[0]
	c.Assert(one.Error, tc.IsNil)

	expected := params.StorageDetails{
		StorageTag: s.storageTag.String(),
		OwnerTag:   s.unitTag.String(),
		Kind:       params.StorageKindFilesystem,
		Life:       "dying",
		Status: params.EntityStatus{
			Status: "attached",
		},
		Attachments: map[string]params.StorageAttachmentDetails{
			s.unitTag.String(): {
				StorageTag: s.storageTag.String(),
				UnitTag:    s.unitTag.String(),
				MachineTag: s.machineTag.String(),
				Location:   "",
				Life:       "alive",
			},
		},
	}
	c.Assert(one.Result, tc.DeepEquals, &expected)
}

func (s *storageSuite) TestShowStorageInvalidId(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageTag := "foo"
	entity := params.Entity{Tag: storageTag}

	found, err := s.api.StorageDetails(c.Context(), params.Entities{Entities: []params.Entity{entity}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 1)
	s.assertInstanceInfoError(c, found.Results[0], params.StorageDetailsResult{}, `"foo" is not a valid tag`)
}

func (s *storageSuite) TestRemove(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.RemoveBlock).Return("", blockcommanderrors.NotFound)
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	results, err := s.api.Remove(c.Context(), params.RemoveStorage{Storage: []params.RemoveStorageInstance{
		{Tag: "storage-foo-0", DestroyStorage: true},
		{Tag: "storage-foo-1", DestroyAttachments: true, DestroyStorage: true},
		{Tag: "storage-foo-1", DestroyAttachments: true, DestroyStorage: false},
		{Tag: "volume-0"},
		{Tag: "filesystem-1-2"},
		{Tag: "machine-0"},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Message: "cannae do it"}},
		{Error: &params.Error{Message: "cannae do it"}},
		{Error: &params.Error{Message: "cannae do it"}},
		{Error: &params.Error{Message: `"volume-0" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"filesystem-1-2" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"machine-0" is not a valid storage tag`}},
	})
	s.stub.CheckCallNames(c,
		destroyStorageInstanceCall,
		destroyStorageInstanceCall,
		releaseStorageInstanceCall,
	)
	s.stub.CheckCall(c, 0, destroyStorageInstanceCall, names.NewStorageTag("foo/0"), false, false)
	s.stub.CheckCall(c, 1, destroyStorageInstanceCall, names.NewStorageTag("foo/1"), true, false)
	s.stub.CheckCall(c, 2, releaseStorageInstanceCall, names.NewStorageTag("foo/1"), true, false)
}

func (s *storageSuite) TestDetach(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	s.applicationService.EXPECT().DetachStorageForUnit(gomock.Any(), corestorage.ID("data/0"), coreunit.Name("mysql/0"))
	s.applicationService.EXPECT().DetachStorageFromUnit(gomock.Any(), corestorage.ID("data/0"))

	results, err := s.api.DetachStorage(
		c.Context(),
		params.StorageDetachmentParams{
			StorageIds: params.StorageAttachmentIds{Ids: []params.StorageAttachmentId{
				{StorageTag: "storage-data-0", UnitTag: "unit-mysql-0"},
				{StorageTag: "storage-data-0", UnitTag: ""},
				{StorageTag: "volume-0", UnitTag: "unit-bar-0"},
				{StorageTag: "filesystem-1-2", UnitTag: "unit-bar-0"},
				{StorageTag: "machine-0", UnitTag: "unit-bar-0"},
				{StorageTag: "storage-foo-0", UnitTag: "application-bar"},
			}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 6)
	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
		{Error: &params.Error{Message: `"volume-0" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"filesystem-1-2" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"machine-0" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"application-bar" is not a valid unit tag`}},
	})
}

func (s *storageSuite) TestDetachSpecifiedNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	s.applicationService.EXPECT().DetachStorageForUnit(gomock.Any(), corestorage.ID("data/0"), coreunit.Name("foo/42")).
		Return(storageerrors.StorageAttachmentNotFound)

	results, err := s.api.DetachStorage(
		c.Context(),
		params.StorageDetachmentParams{
			StorageIds: params.StorageAttachmentIds{Ids: []params.StorageAttachmentId{
				{StorageTag: "storage-data-0", UnitTag: "unit-foo-42"},
			}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "attachment of storage data/0 to unit foo/42 not found",
		}},
	})
}

func (s *storageSuite) TestDetachNoAttachmentsStorageNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	s.applicationService.EXPECT().DetachStorageFromUnit(gomock.Any(), corestorage.ID("foo/42")).Return(storageerrors.StorageNotFound)

	results, err := s.api.DetachStorage(
		c.Context(),
		params.StorageDetachmentParams{
			StorageIds: params.StorageAttachmentIds{Ids: []params.StorageAttachmentId{
				{StorageTag: "storage-foo-42"},
			}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "storage foo/42 not found",
		}},
	})
}

func (s *storageSuite) TestAttach(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	s.applicationService.EXPECT().AttachStorageToUnit(gomock.Any(), corestorage.ID("data/0"), coreunit.Name("mysql/0"))

	results, err := s.api.Attach(c.Context(),
		params.StorageAttachmentIds{Ids: []params.StorageAttachmentId{
			{StorageTag: "storage-data-0", UnitTag: "unit-mysql-0"},
			{StorageTag: "storage-data-0", UnitTag: "machine-0"},
			{StorageTag: "volume-0", UnitTag: "unit-mysql-0"},
		}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 3)
	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: &params.Error{Message: `"machine-0" is not a valid unit tag`}},
		{Error: &params.Error{Message: `"volume-0" is not a valid storage tag`}},
	})
}

func (s *storageSuite) TestImportFilesystem(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	s.storageService.EXPECT().ImportProviderStorage(gomock.Any(), service.ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "radiance",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}).Return("data/0", nil)

	results, err := s.api.Import(c.Context(), params.BulkImportStorageParams{Storage: []params.ImportStorageParams{{
		Kind:        params.StorageKindFilesystem,
		Pool:        "radiance",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.DeepEquals, []params.ImportStorageResult{{
		Result: &params.ImportStorageDetails{
			StorageTag: "storage-data-0",
		},
	}})
}

func (s *storageSuite) TestImportFilesystemVolumeBacked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	s.storageService.EXPECT().ImportProviderStorage(gomock.Any(), service.ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "radiance",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}).Return("", errors.New("importing filesystem: nope"))

	results, err := s.api.Import(c.Context(),
		params.BulkImportStorageParams{Storage: []params.ImportStorageParams{{
			Kind:        params.StorageKindFilesystem,
			Pool:        "radiance",
			ProviderId:  "foo",
			StorageName: "pgdata",
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.DeepEquals, []params.ImportStorageResult{
		{Error: &params.Error{Message: `importing filesystem: nope`}},
	})
}

func (s *storageSuite) TestImportFilesystemNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	s.storageService.EXPECT().ImportProviderStorage(gomock.Any(), service.ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "radiance",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}).Return("", errors.NotSupportedf(`importing filesystem with storage provider "radiance"`))

	results, err := s.api.Import(c.Context(), params.BulkImportStorageParams{Storage: []params.ImportStorageParams{{
		Kind:        params.StorageKindFilesystem,
		Pool:        "radiance",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.DeepEquals, []params.ImportStorageResult{
		{Error: &params.Error{
			Message: `importing filesystem with storage provider "radiance" not supported`,
			Code:    "not supported",
		}},
	})
}

func (s *storageSuite) TestListStorageAsAdminOnNotOwnedModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("superuserfoo"),
	}
	controllerUUID := uuid.MustNewUUID().String()
	modelUUID := modeltesting.GenModelUUID(c)
	s.api = facadestorage.NewStorageAPI(
		controllerUUID, modelUUID, coremodel.IAAS,
		s.storageAccessor, nil, s.storageService,
		s.applicationService,
		s.authorizer, s.blockCommandService)

	// ListStorageDetails should not fail
	_, err := s.api.ListStorageDetails(c.Context(), params.StorageFilters{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestListStorageAsNonAdminOnNotOwnedModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("userfoo"),
	}
	controllerUUID := uuid.MustNewUUID().String()
	modelUUID := modeltesting.GenModelUUID(c)
	s.api = facadestorage.NewStorageAPI(
		controllerUUID, modelUUID, coremodel.IAAS,
		s.storageAccessor, nil, s.storageService,
		s.applicationService,
		s.authorizer, s.blockCommandService)

	// ListStorageDetails should fail with perm error
	_, err := s.api.ListStorageDetails(c.Context(), params.StorageFilters{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}
