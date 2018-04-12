// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/dummy"
	coretesting "github.com/juju/juju/testing"
)

type storageSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) TestStorageListEmpty(c *gc.C) {
	s.state.allStorageInstances = func() ([]state.StorageInstance, error) {
		s.stub.AddCall(allStorageInstancesCall)
		return []state.StorageInstance{}, nil
	}

	found, err := s.api.ListStorageDetails(
		params.StorageFilters{[]params.StorageFilter{{}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.IsNil)
	c.Assert(found.Results[0].Result, gc.HasLen, 0)
	s.assertCalls(c, []string{allStorageInstancesCall})
}

func (s *storageSuite) TestStorageListFilesystem(c *gc.C) {
	found, err := s.api.ListStorageDetails(
		params.StorageFilters{[]params.StorageFilter{{}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceFilesystemCall,
		storageInstanceAttachmentsCall,
		unitAssignedMachineCall,
		storageInstanceCall,
		storageInstanceFilesystemCall,
		storageInstanceFilesystemAttachmentCall,
	}
	s.assertCalls(c, expectedCalls)

	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.IsNil)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	wantedDetails := s.createTestStorageDetails()
	c.Assert(found.Results[0].Result[0], jc.DeepEquals, wantedDetails)
}

func (s *storageSuite) TestStorageListVolume(c *gc.C) {
	s.storageInstance.kind = state.StorageKindBlock
	found, err := s.api.ListStorageDetails(
		params.StorageFilters{[]params.StorageFilter{{}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceVolumeCall,
		storageInstanceAttachmentsCall,
		unitAssignedMachineCall,
		storageInstanceCall,
		storageInstanceVolumeCall,
	}
	s.assertCalls(c, expectedCalls)

	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.IsNil)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	wantedDetails := s.createTestStorageDetails()
	wantedDetails.Kind = params.StorageKindBlock
	wantedDetails.Status.Status = status.Attached
	c.Assert(found.Results[0].Result[0], jc.DeepEquals, wantedDetails)
}

func (s *storageSuite) TestStorageListError(c *gc.C) {
	msg := "list test error"
	s.state.allStorageInstances = func() ([]state.StorageInstance, error) {
		s.stub.AddCall(allStorageInstancesCall)
		return []state.StorageInstance{}, errors.Errorf(msg)
	}

	found, err := s.api.ListStorageDetails(
		params.StorageFilters{[]params.StorageFilter{{}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.ErrorMatches, msg)

	expectedCalls := []string{allStorageInstancesCall}
	s.assertCalls(c, expectedCalls)
}

func (s *storageSuite) TestStorageListInstanceError(c *gc.C) {
	msg := "list test error"
	s.state.storageInstance = func(sTag names.StorageTag) (state.StorageInstance, error) {
		s.stub.AddCall(storageInstanceCall)
		c.Assert(sTag, jc.DeepEquals, s.storageTag)
		return nil, errors.Errorf(msg)
	}

	found, err := s.api.ListStorageDetails(
		params.StorageFilters{[]params.StorageFilter{{}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceFilesystemCall,
		storageInstanceAttachmentsCall,
		unitAssignedMachineCall,
		storageInstanceCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.ErrorMatches,
		fmt.Sprintf("getting details for storage data/0: getting storage instance: %v", msg),
	)
}

func (s *storageSuite) TestStorageListAttachmentError(c *gc.C) {
	s.state.storageInstanceAttachments = func(tag names.StorageTag) ([]state.StorageAttachment, error) {
		s.stub.AddCall(storageInstanceAttachmentsCall)
		c.Assert(tag, jc.DeepEquals, s.storageTag)
		return []state.StorageAttachment{}, errors.Errorf("list test error")
	}

	found, err := s.api.ListStorageDetails(
		params.StorageFilters{[]params.StorageFilter{{}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceFilesystemCall,
		storageInstanceAttachmentsCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.ErrorMatches,
		"getting details for storage data/0: list test error")
}

func (s *storageSuite) TestStorageListMachineError(c *gc.C) {
	msg := "list test error"
	s.state.unitAssignedMachine = func(u names.UnitTag) (names.MachineTag, error) {
		s.stub.AddCall(unitAssignedMachineCall)
		c.Assert(u, jc.DeepEquals, s.unitTag)
		return names.MachineTag{}, errors.Errorf(msg)
	}

	found, err := s.api.ListStorageDetails(
		params.StorageFilters{[]params.StorageFilter{{}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceFilesystemCall,
		storageInstanceAttachmentsCall,
		unitAssignedMachineCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.ErrorMatches,
		fmt.Sprintf("getting details for storage data/0: %v", msg),
	)
}

func (s *storageSuite) TestStorageListFilesystemError(c *gc.C) {
	msg := "list test error"
	s.state.storageInstanceFilesystem = func(sTag names.StorageTag) (state.Filesystem, error) {
		s.stub.AddCall(storageInstanceFilesystemCall)
		c.Assert(sTag, jc.DeepEquals, s.storageTag)
		return nil, errors.Errorf(msg)
	}

	found, err := s.api.ListStorageDetails(
		params.StorageFilters{[]params.StorageFilter{{}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceFilesystemCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.ErrorMatches,
		fmt.Sprintf("getting details for storage data/0: %v", msg),
	)
}

func (s *storageSuite) TestStorageListFilesystemAttachmentError(c *gc.C) {
	msg := "list test error"
	s.state.unitAssignedMachine = func(u names.UnitTag) (names.MachineTag, error) {
		s.stub.AddCall(unitAssignedMachineCall)
		c.Assert(u, jc.DeepEquals, s.unitTag)
		return s.machineTag, errors.Errorf(msg)
	}

	found, err := s.api.ListStorageDetails(
		params.StorageFilters{[]params.StorageFilter{{}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceFilesystemCall,
		storageInstanceAttachmentsCall,
		unitAssignedMachineCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.ErrorMatches,
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
			s.unitTag.String(): params.StorageAttachmentDetails{
				s.storageTag.String(),
				s.unitTag.String(),
				s.machineTag.String(),
				"", // location
				"alive",
			},
		},
	}
}

func (s *storageSuite) assertInstanceInfoError(c *gc.C, obtained params.StorageDetailsResult, wanted params.StorageDetailsResult, expected string) {
	if expected != "" {
		c.Assert(errors.Cause(obtained.Error), gc.ErrorMatches, fmt.Sprintf(".*%v.*", expected))
		c.Assert(obtained.Result, gc.IsNil)
	} else {
		c.Assert(obtained.Error, gc.IsNil)
		c.Assert(obtained, jc.DeepEquals, wanted)
	}
}

func (s *storageSuite) TestShowStorageEmpty(c *gc.C) {
	found, err := s.api.StorageDetails(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 0)
}

func (s *storageSuite) TestShowStorageInvalidTag(c *gc.C) {
	// Only storage tags are permitted
	found, err := s.api.StorageDetails(params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.ErrorMatches, `"machine-1" is not a valid storage tag`)
}

func (s *storageSuite) TestShowStorage(c *gc.C) {
	entity := params.Entity{Tag: s.storageTag.String()}

	found, err := s.api.StorageDetails(
		params.Entities{Entities: []params.Entity{entity}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)

	one := found.Results[0]
	c.Assert(one.Error, gc.IsNil)

	expected := params.StorageDetails{
		StorageTag: s.storageTag.String(),
		OwnerTag:   s.unitTag.String(),
		Kind:       params.StorageKindFilesystem,
		Life:       "dying",
		Status: params.EntityStatus{
			Status: "attached",
		},
		Attachments: map[string]params.StorageAttachmentDetails{
			s.unitTag.String(): params.StorageAttachmentDetails{
				s.storageTag.String(),
				s.unitTag.String(),
				s.machineTag.String(),
				"",
				"alive",
			},
		},
	}
	c.Assert(one.Result, jc.DeepEquals, &expected)
}

func (s *storageSuite) TestShowStorageInvalidId(c *gc.C) {
	storageTag := "foo"
	entity := params.Entity{Tag: storageTag}

	found, err := s.api.StorageDetails(params.Entities{Entities: []params.Entity{entity}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	s.assertInstanceInfoError(c, found.Results[0], params.StorageDetailsResult{}, `"foo" is not a valid tag`)
}

func (s *storageSuite) TestRemove(c *gc.C) {
	results, err := s.api.Remove(params.RemoveStorage{[]params.RemoveStorageInstance{
		{Tag: "storage-foo-0", DestroyStorage: true},
		{Tag: "storage-foo-1", DestroyAttachments: true, DestroyStorage: true},
		{Tag: "storage-foo-1", DestroyAttachments: true, DestroyStorage: false},
		{Tag: "volume-0"},
		{Tag: "filesystem-1-2"},
		{Tag: "machine-0"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Message: "cannae do it"}},
		{Error: &params.Error{Message: "cannae do it"}},
		{Error: &params.Error{Message: "cannae do it"}},
		{Error: &params.Error{Message: `"volume-0" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"filesystem-1-2" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"machine-0" is not a valid storage tag`}},
	})
	s.stub.CheckCallNames(c,
		getBlockForTypeCall, // Remove
		getBlockForTypeCall, // Change
		destroyStorageInstanceCall,
		destroyStorageInstanceCall,
		releaseStorageInstanceCall,
	)
	s.stub.CheckCall(c, 2, destroyStorageInstanceCall, names.NewStorageTag("foo/0"), false)
	s.stub.CheckCall(c, 3, destroyStorageInstanceCall, names.NewStorageTag("foo/1"), true)
	s.stub.CheckCall(c, 4, releaseStorageInstanceCall, names.NewStorageTag("foo/1"), true)
}

func (s *storageSuite) TestDestroyV3(c *gc.C) {
	results, err := s.apiv3.Destroy(params.Entities{[]params.Entity{
		{Tag: "storage-foo-0"},
		{Tag: "volume-0"},
		{Tag: "filesystem-1-2"},
		{Tag: "machine-0"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Message: "cannae do it"}},
		{Error: &params.Error{Message: `"volume-0" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"filesystem-1-2" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"machine-0" is not a valid storage tag`}},
	})
	s.stub.CheckCallNames(c,
		getBlockForTypeCall, // Remove
		getBlockForTypeCall, // Change
		destroyStorageInstanceCall,
	)
	s.stub.CheckCall(c, 2, destroyStorageInstanceCall, names.NewStorageTag("foo/0"), true)
}

func (s *storageSuite) TestDetach(c *gc.C) {
	results, err := s.api.Detach(params.StorageAttachmentIds{[]params.StorageAttachmentId{
		{StorageTag: "storage-data-0", UnitTag: "unit-mysql-0"},
		{StorageTag: "storage-data-0", UnitTag: ""},
		{StorageTag: "volume-0", UnitTag: "unit-bar-0"},
		{StorageTag: "filesystem-1-2", UnitTag: "unit-bar-0"},
		{StorageTag: "machine-0", UnitTag: "unit-bar-0"},
		{StorageTag: "storage-foo-0", UnitTag: "application-bar"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 6)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
		{Error: &params.Error{Message: `"volume-0" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"filesystem-1-2" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"machine-0" is not a valid storage tag`}},
		{Error: &params.Error{Message: `"application-bar" is not a valid unit tag`}},
	})
	s.assertCalls(c, []string{
		getBlockForTypeCall, // Change
		detachStorageCall,
		storageInstanceAttachmentsCall,
		detachStorageCall,
	})
	s.stub.CheckCalls(c, []testing.StubCall{
		{getBlockForTypeCall, []interface{}{state.ChangeBlock}},
		{detachStorageCall, []interface{}{s.storageTag, s.unitTag}},
		{storageInstanceAttachmentsCall, []interface{}{s.storageTag}},
		{detachStorageCall, []interface{}{s.storageTag, s.unitTag}},
	})
}

func (s *storageSuite) TestDetachSpecifiedNotFound(c *gc.C) {
	results, err := s.api.Detach(params.StorageAttachmentIds{[]params.StorageAttachmentId{
		{StorageTag: "storage-data-0", UnitTag: "unit-foo-42"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "attachment of storage data/0 to unit foo/42 not found",
		}},
	})
	s.assertCalls(c, []string{
		getBlockForTypeCall, // Change
		detachStorageCall,
	})
	s.stub.CheckCalls(c, []testing.StubCall{
		{getBlockForTypeCall, []interface{}{state.ChangeBlock}},
		{detachStorageCall, []interface{}{
			s.storageTag,
			names.NewUnitTag("foo/42"),
		}},
	})
}

func (s *storageSuite) TestDetachAttachmentNotFoundConcurrent(c *gc.C) {
	// Simulate:
	//  1. call StorageAttachments, and receive
	//     a list of alive attachments
	//  2. attachment is concurrently destroyed
	//     and removed by another process
	s.state.detachStorage = func(sTag names.StorageTag, uTag names.UnitTag) error {
		s.stub.AddCall(detachStorageCall, sTag, uTag)
		return errors.NotFoundf(
			"attachment of %s to %s",
			names.ReadableString(sTag),
			names.ReadableString(uTag),
		)
	}
	results, err := s.api.Detach(params.StorageAttachmentIds{[]params.StorageAttachmentId{
		{StorageTag: "storage-data-0"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{{}})
	s.assertCalls(c, []string{
		getBlockForTypeCall, // Change
		storageInstanceAttachmentsCall,
		detachStorageCall,
	})
	s.stub.CheckCalls(c, []testing.StubCall{
		{getBlockForTypeCall, []interface{}{state.ChangeBlock}},
		{storageInstanceAttachmentsCall, []interface{}{s.storageTag}},
		{detachStorageCall, []interface{}{s.storageTag, s.unitTag}},
	})
}

func (s *storageSuite) TestDetachNoAttachmentsStorageNotFound(c *gc.C) {
	results, err := s.api.Detach(params.StorageAttachmentIds{[]params.StorageAttachmentId{
		{StorageTag: "storage-foo-42"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "storage foo/42 not found",
		}},
	})
	s.stub.CheckCalls(c, []testing.StubCall{
		{getBlockForTypeCall, []interface{}{state.ChangeBlock}},
		{storageInstanceAttachmentsCall, []interface{}{names.NewStorageTag("foo/42")}},
		{storageInstanceCall, []interface{}{names.NewStorageTag("foo/42")}},
	})
}

func (s *storageSuite) TestAttach(c *gc.C) {
	results, err := s.api.Attach(params.StorageAttachmentIds{[]params.StorageAttachmentId{
		{StorageTag: "storage-data-0", UnitTag: "unit-mysql-0"},
		{StorageTag: "storage-data-0", UnitTag: "machine-0"},
		{StorageTag: "volume-0", UnitTag: "unit-mysql-0"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: &params.Error{Message: `"machine-0" is not a valid unit tag`}},
		{Error: &params.Error{Message: `"volume-0" is not a valid storage tag`}},
	})
	s.stub.CheckCalls(c, []testing.StubCall{
		{getBlockForTypeCall, []interface{}{state.ChangeBlock}},
		{attachStorageCall, []interface{}{s.storageTag, s.unitTag}},
	})
}

func (s *storageSuite) TestImportFilesystem(c *gc.C) {
	s.state.modelTag = coretesting.ModelTag
	filesystemSource := filesystemImporter{&dummy.FilesystemSource{}}
	dummyStorageProvider := &dummy.StorageProvider{
		StorageScope: storage.ScopeEnviron,
		IsDynamic:    true,
		FilesystemSourceFunc: func(*storage.Config) (storage.FilesystemSource, error) {
			return filesystemSource, nil
		},
	}
	s.registry.Providers["radiance"] = dummyStorageProvider

	results, err := s.api.Import(params.BulkImportStorageParams{[]params.ImportStorageParams{{
		Kind:        params.StorageKindFilesystem,
		Pool:        "radiance",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ImportStorageResult{{
		Result: &params.ImportStorageDetails{
			StorageTag: "storage-data-0",
		},
	}})
	filesystemSource.CheckCalls(c, []testing.StubCall{
		{"ImportFilesystem", []interface{}{
			"foo", map[string]string{
				"juju-model-uuid":      "deadbeef-0bad-400d-8000-4b1d0d06f00d",
				"juju-controller-uuid": "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			},
		}},
	})
	s.stub.CheckCalls(c, []testing.StubCall{
		{getBlockForTypeCall, []interface{}{state.ChangeBlock}},
		{addExistingFilesystemCall, []interface{}{
			state.FilesystemInfo{
				FilesystemId: "foo",
				Pool:         "radiance",
				Size:         123,
			},
			(*state.VolumeInfo)(nil),
			"pgdata",
		}},
	})
}

func (s *storageSuite) TestImportFilesystemVolumeBacked(c *gc.C) {
	s.state.modelTag = coretesting.ModelTag
	volumeSource := volumeImporter{&dummy.VolumeSource{}}
	dummyStorageProvider := &dummy.StorageProvider{
		StorageScope: storage.ScopeEnviron,
		IsDynamic:    true,
		SupportsFunc: func(kind storage.StorageKind) bool {
			return kind == storage.StorageKindBlock
		},
		VolumeSourceFunc: func(*storage.Config) (storage.VolumeSource, error) {
			return volumeSource, nil
		},
	}
	s.registry.Providers["radiance"] = dummyStorageProvider

	results, err := s.api.Import(params.BulkImportStorageParams{[]params.ImportStorageParams{{
		Kind:        params.StorageKindFilesystem,
		Pool:        "radiance",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ImportStorageResult{{
		Result: &params.ImportStorageDetails{
			StorageTag: "storage-data-0",
		},
	}})
	volumeSource.CheckCalls(c, []testing.StubCall{
		{"ImportVolume", []interface{}{
			"foo", map[string]string{
				"juju-model-uuid":      "deadbeef-0bad-400d-8000-4b1d0d06f00d",
				"juju-controller-uuid": "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			},
		}},
	})
	s.stub.CheckCalls(c, []testing.StubCall{
		{getBlockForTypeCall, []interface{}{state.ChangeBlock}},
		{addExistingFilesystemCall, []interface{}{
			state.FilesystemInfo{
				Pool: "radiance",
				Size: 123,
			},
			&state.VolumeInfo{
				VolumeId:   "foo",
				Pool:       "radiance",
				Size:       123,
				HardwareId: "hw",
			},
			"pgdata",
		}},
	})
}

func (s *storageSuite) TestImportFilesystemError(c *gc.C) {
	filesystemSource := filesystemImporter{&dummy.FilesystemSource{}}
	dummyStorageProvider := &dummy.StorageProvider{
		StorageScope: storage.ScopeEnviron,
		IsDynamic:    true,
		FilesystemSourceFunc: func(*storage.Config) (storage.FilesystemSource, error) {
			return filesystemSource, nil
		},
	}
	s.registry.Providers["radiance"] = dummyStorageProvider

	filesystemSource.SetErrors(errors.New("nope"))
	results, err := s.api.Import(params.BulkImportStorageParams{[]params.ImportStorageParams{{
		Kind:        params.StorageKindFilesystem,
		Pool:        "radiance",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ImportStorageResult{
		{Error: &params.Error{Message: `importing filesystem: nope`}},
	})
	filesystemSource.CheckCallNames(c, "ImportFilesystem")
	s.stub.CheckCallNames(c, getBlockForTypeCall)
}

func (s *storageSuite) TestImportFilesystemNotSupported(c *gc.C) {
	filesystemSource := &dummy.FilesystemSource{}
	dummyStorageProvider := &dummy.StorageProvider{
		StorageScope: storage.ScopeEnviron,
		IsDynamic:    true,
		FilesystemSourceFunc: func(*storage.Config) (storage.FilesystemSource, error) {
			return filesystemSource, nil
		},
	}
	s.registry.Providers["radiance"] = dummyStorageProvider

	results, err := s.api.Import(params.BulkImportStorageParams{[]params.ImportStorageParams{{
		Kind:        params.StorageKindFilesystem,
		Pool:        "radiance",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ImportStorageResult{
		{Error: &params.Error{
			Message: `importing filesystem with storage provider "radiance" not supported`,
			Code:    "not supported",
		}},
	})
	filesystemSource.CheckNoCalls(c)
	s.stub.CheckCallNames(c, getBlockForTypeCall)
}

func (s *storageSuite) TestImportFilesystemVolumeBackedNotSupported(c *gc.C) {
	volumeSource := &dummy.VolumeSource{}
	dummyStorageProvider := &dummy.StorageProvider{
		StorageScope: storage.ScopeEnviron,
		IsDynamic:    true,
		SupportsFunc: func(kind storage.StorageKind) bool {
			return kind == storage.StorageKindBlock
		},
		VolumeSourceFunc: func(*storage.Config) (storage.VolumeSource, error) {
			return volumeSource, nil
		},
	}
	s.registry.Providers["radiance"] = dummyStorageProvider

	results, err := s.api.Import(params.BulkImportStorageParams{[]params.ImportStorageParams{{
		Kind:        params.StorageKindFilesystem,
		Pool:        "radiance",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ImportStorageResult{
		{Error: &params.Error{
			Message: `importing volume with storage provider "radiance" not supported`,
			Code:    "not supported",
		}},
	})
	volumeSource.CheckNoCalls(c)
	s.stub.CheckCallNames(c, getBlockForTypeCall)
}

func (s *storageSuite) TestImportValidationErrors(c *gc.C) {
	results, err := s.api.Import(params.BulkImportStorageParams{[]params.ImportStorageParams{{
		Kind:        params.StorageKindBlock,
		Pool:        "radiance",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}, {
		Kind:        params.StorageKindFilesystem,
		Pool:        "123",
		ProviderId:  "foo",
		StorageName: "pgdata",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ImportStorageResult{
		{Error: &params.Error{Message: `storage kind "block" not supported`, Code: "not supported"}},
		{Error: &params.Error{Message: `pool name "123" not valid`}},
	})
}

type filesystemImporter struct {
	*dummy.FilesystemSource
}

// ImportFilesystem is part of the storage.FilesystemImporter interface.
func (f filesystemImporter) ImportFilesystem(providerId string, tags map[string]string) (storage.FilesystemInfo, error) {
	f.MethodCall(f, "ImportFilesystem", providerId, tags)
	return storage.FilesystemInfo{
		FilesystemId: providerId,
		Size:         123,
	}, f.NextErr()
}

type volumeImporter struct {
	*dummy.VolumeSource
}

// ImportVolume is part of the storage.VolumeImporter interface.
func (v volumeImporter) ImportVolume(providerId string, tags map[string]string) (storage.VolumeInfo, error) {
	v.MethodCall(v, "ImportVolume", providerId, tags)
	return storage.VolumeInfo{
		VolumeId:   providerId,
		Size:       123,
		HardwareId: "hw",
	}, v.NextErr()
}
