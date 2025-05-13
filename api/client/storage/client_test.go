// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/storage"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	jujustorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

type storageMockSuite struct {
}

var _ = tc.Suite(&storageMockSuite{})

func (s *storageMockSuite) TestStorageDetails(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "StorageDetails", args, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)

	tags := []names.StorageTag{oneTag, twoTag}
	found, err := storageClient.StorageDetails(context.Background(), tags)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.HasLen, 3)
	c.Assert(expected.Contains(found[0].Result.StorageTag), tc.IsTrue)
	c.Assert(expected.Contains(found[1].Result.StorageTag), tc.IsTrue)
	c.Assert(found[2].Error, tc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestStorageDetailsFacadeCallError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)

	result := new(params.StorageDetailsResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "StorageDetails", gomock.AssignableToTypeOf(params.Entities{}), result).SetArg(3, params.StorageDetailsResults{}).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.StorageDetails(context.Background(), []names.StorageTag{oneTag})
	c.Assert(err, tc.ErrorMatches, msg)
	c.Assert(found, tc.HasLen, 0)
}

func (s *storageMockSuite) TestListStorageDetails(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListStorageDetails", gomock.AssignableToTypeOf(params.StorageFilters{}), result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListStorageDetails(context.Background())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(found, tc.HasLen, 1)
	expected := []params.StorageDetails{{
		StorageTag: "storage-db-dir-1000",
		Status: params.EntityStatus{
			Status: "attached",
		},
		Persistent: true,
	}}

	c.Assert(found, tc.DeepEquals, expected)
}

func (s *storageMockSuite) TestListStorageDetailsFacadeCallError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"

	result := new(params.StorageDetailsListResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListStorageDetails", gomock.AssignableToTypeOf(params.StorageFilters{}), result).SetArg(3, params.StorageDetailsListResults{}).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListStorageDetails(context.Background())
	c.Assert(err, tc.ErrorMatches, msg)
	c.Assert(found, tc.HasLen, 0)
}

func (s *storageMockSuite) TestListPools(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListPools", args, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)

	found, err := storageClient.ListPools(context.Background(), types, someNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.HasLen, want)
	c.Assert(found, tc.DeepEquals, expected)
}

func (s *storageMockSuite) TestListPoolsFacadeCallError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"

	result := new(params.StoragePoolsResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListPools", gomock.AssignableToTypeOf(params.StoragePoolFilters{}), result).SetArg(3, params.StoragePoolsResults{}).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListPools(context.Background(), nil, nil)
	c.Assert(err, tc.ErrorMatches, msg)
	c.Assert(found, tc.HasLen, 0)
}

func (s *storageMockSuite) TestCreatePool(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CreatePool", args, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.CreatePool(context.Background(), poolName, poolType, poolConfig)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageMockSuite) TestCreatePoolFacadeCallError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"

	result := new(params.ErrorResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CreatePool", gomock.AssignableToTypeOf(params.StoragePoolArgs{}), result).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.CreatePool(context.Background(), "", "", nil)
	c.Assert(err, tc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestListVolumes(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListVolumes", args, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListVolumes(context.Background(), []string{"0", "1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.HasLen, 2)
	for i := 0; i < 2; i++ {
		c.Assert(found[i].Result, tc.DeepEquals, []params.VolumeDetails{{
			VolumeTag: "volume-0",
			MachineAttachments: map[string]params.VolumeAttachmentDetails{
				"machine-0": {},
				"machine-1": {},
			},
		}})
	}
}

func (s *storageMockSuite) TestListVolumesEmptyFilter(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListVolumes", args, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListVolumes(context.Background(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.HasLen, 1)
	c.Assert(found[0].Result, tc.HasLen, 1)
	c.Assert(found[0].Result[0].VolumeTag, tc.Equals, tag)
}

func (s *storageMockSuite) TestListVolumesFacadeCallError(c *tc.C) {
	msg := "facade failure"

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.VolumeFilters{
		Filters: []params.VolumeFilter{{}},
	}
	result := new(params.VolumeDetailsListResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListVolumes", args, result).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.ListVolumes(context.Background(), nil)
	c.Assert(err, tc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestListFilesystems(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListFilesystems", args, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.ListFilesystems(context.Background(), []string{"1", "2"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.HasLen, 2)
	c.Assert(found[0].Result, tc.DeepEquals, []params.FilesystemDetails{expected})
	c.Assert(found[1].Result, tc.DeepEquals, []params.FilesystemDetails{})
}

func (s *storageMockSuite) TestListFilesystemsEmptyFilter(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListFilesystems", args, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.ListFilesystems(context.Background(), nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageMockSuite) TestListFilesystemsFacadeCallError(c *tc.C) {
	msg := "facade failure"

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.FilesystemFilters{
		Filters: []params.FilesystemFilter{{}},
	}
	result := new(params.FilesystemDetailsListResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListFilesystems", args, result).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.ListFilesystems(context.Background(), nil)
	c.Assert(err, tc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestAddToUnit(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	size := uint64(42)
	directives := params.StorageDirectives{
		Pool: "value",
		Size: &size,
	}

	errOut := "error"
	unitStorages := []params.StorageAddParams{
		{UnitTag: "u-a", StorageName: "one", Directives: directives},
		{UnitTag: "u-b", StorageName: errOut, Directives: directives},
		{UnitTag: "u-b", StorageName: "nil-constraints"},
	}

	storageN := 3
	expectedError := apiservererrors.ServerError(errors.NotValidf("storage directive"))
	expectedDetails := &params.AddStorageDetails{StorageTags: []string{"a/0", "b/1"}}
	one := func(u, s string, attrs params.StorageDirectives) params.AddStorageResult {
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
			one(unitStorages[0].UnitTag, unitStorages[0].StorageName, unitStorages[0].Directives),
			one(unitStorages[1].UnitTag, unitStorages[1].StorageName, unitStorages[1].Directives),
			one(unitStorages[2].UnitTag, unitStorages[2].StorageName, unitStorages[2].Directives),
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddToUnit", args, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	r, err := storageClient.AddToUnit(context.Background(), unitStorages)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r, tc.HasLen, storageN)
	expected := []params.AddStorageResult{
		{Result: expectedDetails},
		{Error: expectedError},
		{Result: expectedDetails},
	}
	c.Assert(r, tc.SameContents, expected)
}

func (s *storageMockSuite) TestAddToUnitFacadeCallError(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddToUnit", args, result).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	found, err := storageClient.AddToUnit(context.Background(), unitStorages)
	c.Assert(err, tc.ErrorMatches, msg)
	c.Assert(found, tc.HasLen, 0)
}

func (s *storageMockSuite) TestRemove(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	false_ := false
	args := params.RemoveStorage{Storage: []params.RemoveStorageInstance{
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Remove", args, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	obtained, err := storageClient.Remove(context.Background(), []string{"foo/0", "bar/1"}, false, false, &false_, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.HasLen, 2)
	c.Assert(obtained[0].Error, tc.IsNil)
	c.Assert(obtained[1].Error, tc.DeepEquals, &params.Error{Message: "baz"})
}

func (s *storageMockSuite) TestRemoveDestroyAttachments(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	true_ := true
	args := params.RemoveStorage{Storage: []params.RemoveStorageInstance{
		{Tag: "storage-foo-0", DestroyAttachments: true, DestroyStorage: true, Force: &true_},
	}}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Remove", args, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	obtained, err := storageClient.Remove(context.Background(), []string{"foo/0"}, true, true, &true_, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.HasLen, 1)
	c.Assert(obtained[0].Error, tc.IsNil)
}

func (s *storageMockSuite) TestRemoveInvalidStorageId(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.Remove(context.Background(), []string{"foo/bar"}, false, false, nil, nil)
	c.Check(err, tc.ErrorMatches, `storage ID "foo/bar" not valid`)
}

func (s *storageMockSuite) TestDetach(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	expectedArgs := params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{Ids: []params.StorageAttachmentId{
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DetachStorage", expectedArgs, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	obtained, err := storageClient.Detach(context.Background(), []string{"foo/0", "bar/1"}, nil, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.HasLen, 2)
	c.Assert(obtained[0].Error, tc.IsNil)
	c.Assert(obtained[1].Error, tc.DeepEquals, &params.Error{Message: "baz"})
}

func (s *storageMockSuite) TestDetachArityMismatch(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}, {}, {}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DetachStorage", gomock.AssignableToTypeOf(params.StorageDetachmentParams{}), result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.Detach(context.Background(), []string{"foo/0", "bar/1"}, nil, nil)
	c.Check(err, tc.ErrorMatches, `expected 2 result\(s\), got 3`)
}

func (s *storageMockSuite) TestAttach(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	expectedArgs := params.StorageAttachmentIds{Ids: []params.StorageAttachmentId{
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Attach", expectedArgs, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	obtained, err := storageClient.Attach(context.Background(), "foo/0", []string{"bar/1", "baz/2"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.HasLen, 2)
	c.Assert(obtained[0].Error, tc.IsNil)
	c.Assert(obtained[1].Error, tc.DeepEquals, &params.Error{Message: "qux"})
}

func (s *storageMockSuite) TestAttachArityMismatch(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}, {}, {}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Attach", gomock.AssignableToTypeOf(params.StorageAttachmentIds{}), result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.Attach(context.Background(), "foo/0", []string{"bar/1", "baz/2"})
	c.Check(err, tc.ErrorMatches, `expected 2 result\(s\), got 3`)
}

func (s *storageMockSuite) TestImport(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	expectedArgs := params.BulkImportStorageParams{Storage: []params.ImportStorageParams{{
		Kind:        params.StorageKindBlock,
		Pool:        "foo",
		ProviderId:  "bar",
		StorageName: "baz",
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Import", expectedArgs, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	storageTag, err := storageClient.Import(context.Background(), jujustorage.StorageKindBlock, "foo", "bar", "baz")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageTag, tc.Equals, names.NewStorageTag("qux/0"))
}

func (s *storageMockSuite) TestImportError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	result := new(params.ImportStorageResults)
	results := params.ImportStorageResults{
		Results: []params.ImportStorageResult{{
			Error: &params.Error{Message: "qux"},
		},
		}}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Import", gomock.AssignableToTypeOf(params.BulkImportStorageParams{}), result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.Import(context.Background(), jujustorage.StorageKindBlock, "foo", "bar", "baz")
	c.Check(err, tc.ErrorMatches, "qux")
}

func (s *storageMockSuite) TestImportArityMismatch(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.ImportStorageResults)
	results := params.ImportStorageResults{
		Results: []params.ImportStorageResult{{}, {}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Import", gomock.AssignableToTypeOf(params.BulkImportStorageParams{}), result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	_, err := storageClient.Import(context.Background(), jujustorage.StorageKindBlock, "foo", "bar", "baz")
	c.Check(err, tc.ErrorMatches, `expected 1 result, got 2`)
}

func (s *storageMockSuite) TestRemovePool(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "RemovePool", expectedArgs, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.RemovePool(context.Background(), poolName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageMockSuite) TestRemovePoolFacadeCallError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 1),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "RemovePool", gomock.AssignableToTypeOf(params.StoragePoolDeleteArgs{}), result).SetArg(3, results).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.RemovePool(context.Background(), "")
	c.Assert(err, tc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestUpdatePool(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdatePool", expectedArgs, result).SetArg(3, results).Return(nil)

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.UpdatePool(context.Background(), poolName, providerType, poolConfig)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageMockSuite) TestUpdatePoolFacadeCallError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 1),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdatePool", gomock.AssignableToTypeOf(params.StoragePoolArgs{}), result).SetArg(3, results).Return(errors.New(msg))

	storageClient := storage.NewClientFromCaller(mockFacadeCaller)
	err := storageClient.UpdatePool(context.Background(), "", "", nil)
	c.Assert(err, tc.ErrorMatches, msg)
}
