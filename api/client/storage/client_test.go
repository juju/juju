// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/mocks"
	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/storage"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
	jujustorage "github.com/juju/juju/storage"
)

type storageMockSuite struct {
}

var _ = gc.Suite(&storageMockSuite{})

func (s *storageMockSuite) TestStorageDetails(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)
	two := "db-dir/1000"
	twoTag := names.NewStorageTag(two)
	expected := set.NewStrings(oneTag.String(), twoTag.String())
	msg := "call failure"
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: oneTag.String()},
			{Tag: twoTag.String()},
		},
	}
	instances := []params.StorageDetailsResult{
		{
			Result: &params.StorageDetails{StorageTag: oneTag.String()},
		},
		{
			Result: &params.StorageDetails{
				StorageTag: twoTag.String(),
				Status: params.EntityStatus{
					Status: "attached",
				},
				Persistent: true,
			},
		},
		{
			Error: apiservererrors.ServerError(errors.New(msg)),
		},
	}
	results := params.StorageDetailsResults{
		Results: instances,
	}
	result := new(params.StorageDetailsResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("StorageDetails", args, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)

	tags := []names.StorageTag{oneTag, twoTag}
	found, err := storageClient.StorageDetails(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 3)
	c.Assert(expected.Contains(found[0].Result.StorageTag), jc.IsTrue)
	c.Assert(expected.Contains(found[1].Result.StorageTag), jc.IsTrue)
	c.Assert(found[2].Error, gc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestStorageDetailsFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)

	result := new(params.StorageDetailsResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("StorageDetails", gomock.AssignableToTypeOf(params.Entities{}), result).SetArg(2, params.StorageDetailsResults{}).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.StorageDetails([]names.StorageTag{oneTag})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
}

func (s *storageMockSuite) TestListStorageDetails(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	storageTag := names.NewStorageTag("db-dir/1000")
	result := new(params.StorageDetailsListResults)
	results := params.StorageDetailsListResults{
		Results: []params.StorageDetailsListResult{{
			Result: []params.StorageDetails{{
				StorageTag: storageTag.String(),
				Status: params.EntityStatus{
					Status: "attached",
				},
				Persistent: true,
			}},
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListStorageDetails", gomock.AssignableToTypeOf(params.StorageFilters{}), result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListStorageDetails()
	c.Check(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	expected := []params.StorageDetails{{
		StorageTag: "storage-db-dir-1000",
		Status: params.EntityStatus{
			Status: "attached",
		},
		Persistent: true,
	}}

	c.Assert(found, jc.DeepEquals, expected)
}

func (s *storageMockSuite) TestListStorageDetailsFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"

	result := new(params.StorageDetailsListResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListStorageDetails", gomock.AssignableToTypeOf(params.StorageFilters{}), result).SetArg(2, params.StorageDetailsListResults{}).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListStorageDetails()
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
}

func (s *storageMockSuite) TestListPools(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	someNames := []string{"a", "b"}
	types := []string{"1"}
	expected := []params.StoragePool{
		{Name: "name0", Provider: "type0"},
		{Name: "name1", Provider: "type1"},
		{Name: "name2", Provider: "type2"},
	}
	want := len(expected)

	args := params.StoragePoolFilters{
		Filters: []params.StoragePoolFilter{{
			Names:     someNames,
			Providers: types,
		}},
	}
	result := new(params.StoragePoolsResults)
	pools := make([]params.StoragePool, want)
	for i := 0; i < want; i++ {
		pools[i] = params.StoragePool{
			Name:     fmt.Sprintf("name%v", i),
			Provider: fmt.Sprintf("type%v", i),
		}
	}
	results := params.StoragePoolsResults{
		Results: []params.StoragePoolsResult{{
			Result: pools,
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListPools", args, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)

	found, err := storageClient.ListPools(types, someNames)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, want)
	c.Assert(found, gc.DeepEquals, expected)
}

func (s *storageMockSuite) TestListPoolsFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"

	result := new(params.StoragePoolsResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListPools", gomock.AssignableToTypeOf(params.StoragePoolFilters{}), result).SetArg(2, params.StoragePoolsResults{}).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListPools(nil, nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
}

func (s *storageMockSuite) TestCreatePool(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	poolName := "poolName"
	poolType := "poolType"
	poolConfig := map[string]interface{}{
		"test": "one",
		"pass": true,
	}
	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name:     poolName,
			Provider: poolType,
			Attrs:    poolConfig,
		},
		}}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Pools)),
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("CreatePool", args, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.CreatePool(poolName, poolType, poolConfig)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageMockSuite) TestCreatePoolFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"

	result := new(params.ErrorResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("CreatePool", gomock.AssignableToTypeOf(params.StoragePoolArgs{}), result).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.CreatePool("", "", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestListVolumes(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.VolumeFilters{
		Filters: []params.VolumeFilter{
			{Machines: []string{"machine-0"}},
			{Machines: []string{"machine-1"}},
		}}
	result := new(params.VolumeDetailsListResults)
	details := params.VolumeDetails{
		VolumeTag: "volume-0",
		MachineAttachments: map[string]params.VolumeAttachmentDetails{
			"machine-0": {},
			"machine-1": {},
		},
	}
	results := params.VolumeDetailsListResults{
		Results: []params.VolumeDetailsListResult{{
			Result: []params.VolumeDetails{details},
		}, {
			Result: []params.VolumeDetails{details},
		}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListVolumes", args, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListVolumes([]string{"0", "1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 2)
	for i := 0; i < 2; i++ {
		c.Assert(found[i].Result, jc.DeepEquals, []params.VolumeDetails{{
			VolumeTag: "volume-0",
			MachineAttachments: map[string]params.VolumeAttachmentDetails{
				"machine-0": {},
				"machine-1": {},
			},
		}})
	}
}

func (s *storageMockSuite) TestListVolumesEmptyFilter(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag := "ok"
	args := params.VolumeFilters{
		Filters: []params.VolumeFilter{{}},
	}
	result := new(params.VolumeDetailsListResults)
	results := params.VolumeDetailsListResults{
		Results: []params.VolumeDetailsListResult{
			{Result: []params.VolumeDetails{{VolumeTag: tag}}},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListVolumes", args, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListVolumes(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0].Result, gc.HasLen, 1)
	c.Assert(found[0].Result[0].VolumeTag, gc.Equals, tag)
}

func (s *storageMockSuite) TestListVolumesFacadeCallError(c *gc.C) {
	msg := "facade failure"

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.VolumeFilters{
		Filters: []params.VolumeFilter{{}},
	}
	result := new(params.VolumeDetailsListResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListVolumes", args, result).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.ListVolumes(nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestListFilesystems(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.FilesystemFilters{
		Filters: []params.FilesystemFilter{{
			Machines: []string{"machine-1"},
		}, {
			Machines: []string{"machine-2"},
		}},
	}
	result := new(params.FilesystemDetailsListResults)
	expected := params.FilesystemDetails{
		FilesystemTag: "filesystem-1",
		Info: params.FilesystemInfo{
			FilesystemId: "fs-id",
			Size:         4096,
		},
		Status: params.EntityStatus{
			Status: "attached",
		},
		MachineAttachments: map[string]params.FilesystemAttachmentDetails{
			"0": {
				FilesystemAttachmentInfo: params.FilesystemAttachmentInfo{
					MountPoint: "/mnt/kinabalu",
					ReadOnly:   false,
				},
			},
		},
	}
	results := params.FilesystemDetailsListResults{
		Results: []params.FilesystemDetailsListResult{
			{Result: []params.FilesystemDetails{expected}},
			{},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListFilesystems", args, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListFilesystems([]string{"1", "2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 2)
	c.Assert(found[0].Result, jc.DeepEquals, []params.FilesystemDetails{expected})
	c.Assert(found[1].Result, jc.DeepEquals, []params.FilesystemDetails{})
}

func (s *storageMockSuite) TestListFilesystemsEmptyFilter(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.FilesystemFilters{
		Filters: []params.FilesystemFilter{{}},
	}
	result := new(params.FilesystemDetailsListResults)
	results := params.FilesystemDetailsListResults{
		Results: []params.FilesystemDetailsListResult{{}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListFilesystems", args, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.ListFilesystems(nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageMockSuite) TestListFilesystemsFacadeCallError(c *gc.C) {
	msg := "facade failure"

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.FilesystemFilters{
		Filters: []params.FilesystemFilter{{}},
	}
	result := new(params.FilesystemDetailsListResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListFilesystems", args, result).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.ListFilesystems(nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestAddToUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	size := uint64(42)
	cons := params.StorageConstraints{
		Pool: "value",
		Size: &size,
	}

	errOut := "error"
	unitStorages := []params.StorageAddParams{
		{UnitTag: "u-a", StorageName: "one", Constraints: cons},
		{UnitTag: "u-b", StorageName: errOut, Constraints: cons},
		{UnitTag: "u-b", StorageName: "nil-constraints"},
	}

	storageN := 3
	expectedError := apiservererrors.ServerError(errors.NotValidf("storage directive"))
	expectedDetails := &params.AddStorageDetails{[]string{"a/0", "b/1"}}
	one := func(u, s string, attrs params.StorageConstraints) params.AddStorageResult {
		result := params.AddStorageResult{}
		if s == errOut {
			result.Error = expectedError
		} else {
			result.Result = expectedDetails
		}
		return result
	}
	args := params.StoragesAddParams{
		Storages: unitStorages,
	}
	result := new(params.AddStorageResults)
	results := params.AddStorageResults{
		Results: []params.AddStorageResult{
			one(unitStorages[0].UnitTag, unitStorages[0].StorageName, unitStorages[0].Constraints),
			one(unitStorages[1].UnitTag, unitStorages[1].StorageName, unitStorages[1].Constraints),
			one(unitStorages[2].UnitTag, unitStorages[2].StorageName, unitStorages[2].Constraints),
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("AddToUnit", args, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	r, err := storageClient.AddToUnit(unitStorages)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.HasLen, storageN)
	expected := []params.AddStorageResult{
		{Result: expectedDetails},
		{Error: expectedError},
		{Result: expectedDetails},
	}
	c.Assert(r, jc.SameContents, expected)
}

func (s *storageMockSuite) TestAddToUnitFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unitStorages := []params.StorageAddParams{
		{UnitTag: "u-a", StorageName: "one"},
	}
	msg := "facade failure"
	args := params.StoragesAddParams{
		Storages: unitStorages,
	}
	result := new(params.AddStorageResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("AddToUnit", args, result).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.AddToUnit(unitStorages)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
}

func (s *storageMockSuite) TestRemove(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	false_ := false
	args := params.RemoveStorage{[]params.RemoveStorageInstance{
		{Tag: "storage-foo-0", DestroyAttachments: false, DestroyStorage: false, Force: &false_},
		{Tag: "storage-bar-1", DestroyAttachments: false, DestroyStorage: false, Force: &false_},
	}}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: &params.Error{Message: "baz"}},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Remove", args, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	obtained, err := storageClient.Remove([]string{"foo/0", "bar/1"}, false, false, &false_, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.HasLen, 2)
	c.Assert(obtained[0].Error, gc.IsNil)
	c.Assert(obtained[1].Error, jc.DeepEquals, &params.Error{Message: "baz"})
}

func (s *storageMockSuite) TestRemoveDestroyAttachments(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	true_ := true
	args := params.RemoveStorage{[]params.RemoveStorageInstance{
		{Tag: "storage-foo-0", DestroyAttachments: true, DestroyStorage: true, Force: &true_},
	}}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Remove", args, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	obtained, err := storageClient.Remove([]string{"foo/0"}, true, true, &true_, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.HasLen, 1)
	c.Assert(obtained[0].Error, gc.IsNil)
}

func (s *storageMockSuite) TestRemoveInvalidStorageId(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.Remove([]string{"foo/bar"}, false, false, nil, nil)
	c.Check(err, gc.ErrorMatches, `storage ID "foo/bar" not valid`)
}

func (s *storageMockSuite) TestDetach(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	expectedArgs := params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{[]params.StorageAttachmentId{
			{StorageTag: "storage-foo-0"},
			{StorageTag: "storage-bar-1"},
		}},
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: &params.Error{Message: "baz"}},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("DetachStorage", expectedArgs, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	obtained, err := storageClient.Detach([]string{"foo/0", "bar/1"}, nil, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.HasLen, 2)
	c.Assert(obtained[0].Error, gc.IsNil)
	c.Assert(obtained[1].Error, jc.DeepEquals, &params.Error{Message: "baz"})
}

func (s *storageMockSuite) TestDetachArityMismatch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}, {}, {}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("DetachStorage", gomock.AssignableToTypeOf(params.StorageDetachmentParams{}), result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.Detach([]string{"foo/0", "bar/1"}, nil, nil)
	c.Check(err, gc.ErrorMatches, `expected 2 result\(s\), got 3`)
}

func (s *storageMockSuite) TestAttach(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	expectedArgs := params.StorageAttachmentIds{[]params.StorageAttachmentId{
		{
			StorageTag: "storage-bar-1",
			UnitTag:    "unit-foo-0",
		},
		{
			StorageTag: "storage-baz-2",
			UnitTag:    "unit-foo-0",
		},
	}}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: &params.Error{Message: "qux"}},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Attach", expectedArgs, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	obtained, err := storageClient.Attach("foo/0", []string{"bar/1", "baz/2"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.HasLen, 2)
	c.Assert(obtained[0].Error, gc.IsNil)
	c.Assert(obtained[1].Error, jc.DeepEquals, &params.Error{Message: "qux"})
}

func (s *storageMockSuite) TestAttachArityMismatch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}, {}, {}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Attach", gomock.AssignableToTypeOf(params.StorageAttachmentIds{}), result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.Attach("foo/0", []string{"bar/1", "baz/2"})
	c.Check(err, gc.ErrorMatches, `expected 2 result\(s\), got 3`)
}

func (s *storageMockSuite) TestImport(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	expectedArgs := params.BulkImportStorageParams{[]params.ImportStorageParams{{
		Kind:        params.StorageKindBlock,
		Pool:        "foo",
		ProviderId:  "bar",
		StorageName: "baz",
		Force:       false,
	}}}
	result := new(params.ImportStorageResults)
	results := params.ImportStorageResults{
		Results: []params.ImportStorageResult{{
			Result: &params.ImportStorageDetails{
				StorageTag: "storage-qux-0",
			},
		},
		}}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Import", expectedArgs, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)

	mockClientFacade := mocks.NewMockClientFacade(ctrl)
	mockClientFacade.EXPECT().BestAPIVersion().Return(7).AnyTimes()
	storageClient.ClientFacade = mockClientFacade

	storageTag, err := storageClient.Import(jujustorage.StorageKindBlock, "foo", "bar", "baz", false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageTag, gc.Equals, names.NewStorageTag("qux/0"))
}

func (s *storageMockSuite) TestImportError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	result := new(params.ImportStorageResults)
	results := params.ImportStorageResults{
		Results: []params.ImportStorageResult{{
			Error: &params.Error{Message: "qux"},
		},
		}}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Import", gomock.AssignableToTypeOf(params.BulkImportStorageParams{}), result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)

	mockClientFacade := mocks.NewMockClientFacade(ctrl)
	mockClientFacade.EXPECT().BestAPIVersion().Return(7).AnyTimes()
	storageClient.ClientFacade = mockClientFacade

	_, err := storageClient.Import(jujustorage.StorageKindBlock, "foo", "bar", "baz", false)
	c.Check(err, gc.ErrorMatches, "qux")
}

func (s *storageMockSuite) TestImportArityMismatch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.ImportStorageResults)
	results := params.ImportStorageResults{
		Results: []params.ImportStorageResult{{}, {}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Import", gomock.AssignableToTypeOf(params.BulkImportStorageParams{}), result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)

	mockClientFacade := mocks.NewMockClientFacade(ctrl)
	mockClientFacade.EXPECT().BestAPIVersion().Return(7).AnyTimes()
	storageClient.ClientFacade = mockClientFacade

	_, err := storageClient.Import(jujustorage.StorageKindBlock, "foo", "bar", "baz", false)
	c.Check(err, gc.ErrorMatches, `expected 1 result, got 2`)
}

func (s *storageMockSuite) TestImportWithForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	expectedArgs := params.BulkImportStorageParams{[]params.ImportStorageParams{{
		Kind:        params.StorageKindFilesystem,
		Pool:        "kubernetes",
		ProviderId:  "pv-data-001",
		StorageName: "pgdata",
		Force:       true,
	}}}
	result := new(params.ImportStorageResults)
	results := params.ImportStorageResults{
		Results: []params.ImportStorageResult{{
			Result: &params.ImportStorageDetails{
				StorageTag: "storage-pgdata-0",
			},
		},
		}}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Import", expectedArgs, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)

	mockClientFacade := mocks.NewMockClientFacade(ctrl)
	mockClientFacade.EXPECT().BestAPIVersion().Return(7).AnyTimes()
	storageClient.ClientFacade = mockClientFacade

	storageTag, err := storageClient.Import(jujustorage.StorageKindFilesystem, "kubernetes", "pv-data-001", "pgdata", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageTag, gc.Equals, names.NewStorageTag("pgdata/0"))
}

func (s *storageMockSuite) TestImportWithForceAPIVersionNotSupported(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	mockClientFacade := mocks.NewMockClientFacade(ctrl)
	mockClientFacade.EXPECT().BestAPIVersion().Return(6).AnyTimes()
	storageClient.ClientFacade = mockClientFacade

	storageTag, err := storageClient.Import(jujustorage.StorageKindFilesystem, "kubernetes", "pv-data-001", "pgdata", true)
	c.Assert(err, gc.ErrorMatches, "Force import filesystem on this version of Juju not supported")
	c.Assert(storageTag, gc.Equals, names.StorageTag{})
}

func (s *storageMockSuite) TestRemovePool(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	poolName := "poolName"
	expectedArgs := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(expectedArgs.Pools)),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("RemovePool", expectedArgs, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.RemovePool(poolName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageMockSuite) TestRemovePoolFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 1),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("RemovePool", gomock.AssignableToTypeOf(params.StoragePoolDeleteArgs{}), result).SetArg(2, results).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.RemovePool("")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestUpdatePool(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	poolName := "poolName"
	providerType := "loop"
	poolConfig := map[string]interface{}{
		"test": "one",
		"pass": true,
	}
	expectedArgs := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name:     poolName,
			Provider: providerType,
			Attrs:    poolConfig,
		}},
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(expectedArgs.Pools)),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("UpdatePool", expectedArgs, result).SetArg(2, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.UpdatePool(poolName, providerType, poolConfig)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageMockSuite) TestUpdatePoolFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 1),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("UpdatePool", gomock.AssignableToTypeOf(params.StoragePoolArgs{}), result).SetArg(2, results).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.UpdatePool("", "", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}
