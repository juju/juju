// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujustorage "github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type storageMockSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageMockSuite{})

func (s *storageMockSuite) TestStorageDetails(c *gc.C) {
	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)
	two := "db-dir/1000"
	twoTag := names.NewStorageTag(two)
	expected := set.NewStrings(oneTag.String(), twoTag.String())
	msg := "call failure"

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "StorageDetails")

			args, ok := a.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Entities, gc.HasLen, 2)

			if results, k := result.(*params.StorageDetailsResults); k {
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
						Error: common.ServerError(errors.New(msg)),
					},
				}
				results.Results = instances
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	tags := []names.StorageTag{oneTag, twoTag}
	found, err := storageClient.StorageDetails(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 3)
	c.Assert(expected.Contains(found[0].Result.StorageTag), jc.IsTrue)
	c.Assert(expected.Contains(found[1].Result.StorageTag), jc.IsTrue)
	c.Assert(found[2].Error, gc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestStorageDetailsFacadeCallError(c *gc.C) {
	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)

	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "StorageDetails")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.StorageDetails([]names.StorageTag{oneTag})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
}

func (s *storageMockSuite) TestListStorageDetails(c *gc.C) {
	storageTag := names.NewStorageTag("db-dir/1000")

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListStorageDetails")
			c.Check(a, jc.DeepEquals, params.StorageFilters{
				[]params.StorageFilter{{}},
			})

			c.Assert(result, gc.FitsTypeOf, &params.StorageDetailsListResults{})
			results := result.(*params.StorageDetailsListResults)
			results.Results = []params.StorageDetailsListResult{{
				Result: []params.StorageDetails{{
					StorageTag: storageTag.String(),
					Status: params.EntityStatus{
						Status: "attached",
					},
					Persistent: true,
				}},
			}}

			return nil
		},
	)
	storageClient := storage.NewClient(apiCaller)
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
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListStorageDetails")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.ListStorageDetails()
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
}

func (s *storageMockSuite) TestListPools(c *gc.C) {
	expected := []params.StoragePool{
		{Name: "name0", Provider: "type0"},
		{Name: "name1", Provider: "type1"},
		{Name: "name2", Provider: "type2"},
	}
	want := len(expected)

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListPools")

			args := a.(params.StoragePoolFilters)
			c.Assert(args.Filters, gc.HasLen, 1)
			c.Assert(args.Filters[0].Names, gc.HasLen, 2)
			c.Assert(args.Filters[0].Providers, gc.HasLen, 1)

			results := result.(*params.StoragePoolsResults)
			pools := make([]params.StoragePool, want)
			for i := 0; i < want; i++ {
				pools[i] = params.StoragePool{
					Name:     fmt.Sprintf("name%v", i),
					Provider: fmt.Sprintf("type%v", i),
				}
			}
			results.Results = []params.StoragePoolsResult{{
				Result: pools,
			}}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	someNames := []string{"a", "b"}
	types := []string{"1"}
	found, err := storageClient.ListPools(types, someNames)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, want)
	c.Assert(found, gc.DeepEquals, expected)
}

func (s *storageMockSuite) TestListPoolsFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListPools")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.ListPools(nil, nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
}

func (s *storageMockSuite) TestCreatePool(c *gc.C) {
	var called bool
	poolName := "poolName"
	poolType := "poolType"
	poolConfig := map[string]interface{}{
		"test": "one",
		"pass": true,
	}
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				called = true
				c.Check(objType, gc.Equals, "Storage")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "CreatePool")

				args, ok := a.(params.StoragePoolArgs)
				c.Assert(ok, jc.IsTrue)
				c.Assert(args.Pools, gc.HasLen, 1)

				c.Assert(args.Pools[0].Name, gc.Equals, poolName)
				c.Assert(args.Pools[0].Provider, gc.Equals, poolType)
				c.Assert(args.Pools[0].Attrs, gc.DeepEquals, poolConfig)
				results := result.(*params.ErrorResults)

				results.Results = make([]params.ErrorResult, len(args.Pools))
				return nil
			},
		),
		BestVersion: 5,
	}
	storageClient := storage.NewClient(apiCaller)
	err := storageClient.CreatePool(poolName, poolType, poolConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestCreatePoolFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "CreatePool")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	err := storageClient.CreatePool("", "", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestLegacyCreatePool(c *gc.C) {
	var called bool
	poolName := "poolName"
	poolType := "poolType"
	poolConfig := map[string]interface{}{
		"test": "one",
		"pass": true,
	}

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "CreatePool")

			args, ok := a.(params.StoragePool)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Name, gc.Equals, poolName)
			c.Assert(args.Provider, gc.Equals, poolType)
			c.Assert(args.Attrs, gc.DeepEquals, poolConfig)

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	err := storageClient.CreatePool(poolName, poolType, poolConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestLegacyCreatePoolFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "CreatePool")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	err := storageClient.CreatePool("", "", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestListVolumes(c *gc.C) {
	var called bool
	machines := []string{"0", "1"}
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListVolumes")

			c.Assert(a, gc.FitsTypeOf, params.VolumeFilters{})
			args := a.(params.VolumeFilters)
			c.Assert(args.Filters, gc.HasLen, 2)
			c.Assert(args.Filters[0].Machines, jc.DeepEquals, []string{"machine-0"})
			c.Assert(args.Filters[1].Machines, jc.DeepEquals, []string{"machine-1"})

			c.Assert(result, gc.FitsTypeOf, &params.VolumeDetailsListResults{})
			results := result.(*params.VolumeDetailsListResults)

			details := params.VolumeDetails{
				VolumeTag: "volume-0",
				MachineAttachments: map[string]params.VolumeAttachmentDetails{
					"machine-0": {},
					"machine-1": {},
				},
			}
			results.Results = []params.VolumeDetailsListResult{{
				Result: []params.VolumeDetails{details},
			}, {
				Result: []params.VolumeDetails{details},
			}}
			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.ListVolumes(machines)
	c.Assert(called, jc.IsTrue)
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
	var called bool
	tag := "ok"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListVolumes")

			c.Assert(a, gc.FitsTypeOf, params.VolumeFilters{})
			args := a.(params.VolumeFilters)
			c.Assert(args.Filters, gc.HasLen, 1)
			c.Assert(args.Filters[0].IsEmpty(), jc.IsTrue)

			c.Assert(result, gc.FitsTypeOf, &params.VolumeDetailsListResults{})
			results := result.(*params.VolumeDetailsListResults)
			results.Results = []params.VolumeDetailsListResult{
				{Result: []params.VolumeDetails{{VolumeTag: tag}}},
			}
			return nil
		},
	)
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.ListVolumes(nil)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0].Result, gc.HasLen, 1)
	c.Assert(found[0].Result[0].VolumeTag, gc.Equals, tag)
}

func (s *storageMockSuite) TestListVolumesFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListVolumes")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	_, err := storageClient.ListVolumes(nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestListFilesystems(c *gc.C) {
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

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListFilesystems")

			c.Assert(a, gc.FitsTypeOf, params.FilesystemFilters{})
			args := a.(params.FilesystemFilters)
			c.Assert(args.Filters, jc.DeepEquals, []params.FilesystemFilter{{
				Machines: []string{"machine-1"},
			}, {
				Machines: []string{"machine-2"},
			}})

			c.Assert(result, gc.FitsTypeOf, &params.FilesystemDetailsListResults{})
			results := result.(*params.FilesystemDetailsListResults)
			results.Results = []params.FilesystemDetailsListResult{{
				Result: []params.FilesystemDetails{expected},
			}, {}}
			return nil
		},
	)
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.ListFilesystems([]string{"1", "2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 2)
	c.Assert(found[0].Result, jc.DeepEquals, []params.FilesystemDetails{expected})
	c.Assert(found[1].Result, jc.DeepEquals, []params.FilesystemDetails{})
}

func (s *storageMockSuite) TestListFilesystemsEmptyFilter(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListFilesystems")

			c.Assert(a, gc.FitsTypeOf, params.FilesystemFilters{})
			args := a.(params.FilesystemFilters)
			c.Assert(args.Filters, gc.HasLen, 1)
			c.Assert(args.Filters[0].IsEmpty(), jc.IsTrue)

			c.Assert(result, gc.FitsTypeOf, &params.FilesystemDetailsListResults{})
			results := result.(*params.FilesystemDetailsListResults)
			results.Results = []params.FilesystemDetailsListResult{{}}

			return nil
		},
	)
	storageClient := storage.NewClient(apiCaller)
	_, err := storageClient.ListFilesystems(nil)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageMockSuite) TestListFilesystemsFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListFilesystems")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	_, err := storageClient.ListFilesystems(nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestAddToUnit(c *gc.C) {
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
	expectedError := common.ServerError(errors.NotValidf("storage directive"))
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

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "AddToUnit")

			args, ok := a.(params.StoragesAddParams)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Storages, gc.HasLen, storageN)
			c.Assert(args.Storages, gc.DeepEquals, unitStorages)

			if results, k := result.(*params.AddStorageResults); k {
				out := []params.AddStorageResult{}
				for _, s := range args.Storages {
					out = append(out, one(s.UnitTag, s.StorageName, s.Constraints))
				}
				results.Results = out
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
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
	unitStorages := []params.StorageAddParams{
		{UnitTag: "u-a", StorageName: "one"},
	}

	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "AddToUnit")
			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.AddToUnit(unitStorages)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
}

func (s *storageMockSuite) assertRemove(c *gc.C, v int, expectedArgs interface{}) {
	false_ := false
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Storage")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "Remove")
				c.Check(a, jc.DeepEquals, expectedArgs)
				c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
				results := result.(*params.ErrorResults)
				results.Results = []params.ErrorResult{
					{},
					{Error: &params.Error{Message: "baz"}},
				}
				return nil
			},
		),
		BestVersion: v,
	}
	client := storage.NewClient(apiCaller)
	results, err := client.Remove([]string{"foo/0", "bar/1"}, false, false, &false_, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results[0].Error, gc.IsNil)
	c.Assert(results[1].Error, jc.DeepEquals, &params.Error{Message: "baz"})
}

func (s *storageMockSuite) TestRemoveV4(c *gc.C) {
	s.assertRemove(c, 4,
		params.RemoveStorage{[]params.RemoveStorageInstance{
			{Tag: "storage-foo-0", DestroyAttachments: false, DestroyStorage: false},
			{Tag: "storage-bar-1", DestroyAttachments: false, DestroyStorage: false},
		}},
	)
}

func (s *storageMockSuite) TestRemoveV6(c *gc.C) {
	false_ := false
	s.assertRemove(c, 6,
		params.RemoveStorage{[]params.RemoveStorageInstance{
			{Tag: "storage-foo-0", DestroyAttachments: false, DestroyStorage: false, Force: &false_},
			{Tag: "storage-bar-1", DestroyAttachments: false, DestroyStorage: false, Force: &false_},
		}},
	)
}

func (s *storageMockSuite) assertRemoveDestroyAttachments(c *gc.C, v int, expectedArgs interface{}) {
	true_ := true
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(a, jc.DeepEquals, expectedArgs)
				results := result.(*params.ErrorResults)
				results.Results = []params.ErrorResult{{}}
				return nil
			},
		),
		BestVersion: v,
	}
	client := storage.NewClient(apiCaller)
	results, err := client.Remove([]string{"foo/0"}, true, true, &true_, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.IsNil)
}

func (s *storageMockSuite) TestRemoveDestroyAttachmentsv4(c *gc.C) {
	s.assertRemoveDestroyAttachments(c, 4,
		params.RemoveStorage{[]params.RemoveStorageInstance{{
			Tag:                "storage-foo-0",
			DestroyAttachments: true,
			DestroyStorage:     true,
		}}},
	)
}

func (s *storageMockSuite) TestRemoveDestroyAttachmentsv6(c *gc.C) {
	true_ := true
	s.assertRemoveDestroyAttachments(c, 6,
		params.RemoveStorage{[]params.RemoveStorageInstance{{
			Tag:                "storage-foo-0",
			DestroyAttachments: true,
			DestroyStorage:     true,
			Force:              &true_,
		}}},
	)
}

func (s *storageMockSuite) TestRemoveDestroyV3(c *gc.C) {
	false_ := false
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Storage")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "Destroy")
				c.Check(a, jc.DeepEquals, params.Entities{[]params.Entity{
					{Tag: "storage-foo-0"},
				}})
				results := result.(*params.ErrorResults)
				results.Results = []params.ErrorResult{{}}
				return nil
			},
		),
		BestVersion: 3,
	}
	client := storage.NewClient(apiCaller)
	results, err := client.Remove([]string{"foo/0"}, true, true, &false_, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.IsNil)
}

func (s *storageMockSuite) TestRemoveDestroyV3NoDestroyStorage(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{BestVersion: 3}
	client := storage.NewClient(apiCaller)
	_, err := client.Remove([]string{"foo/0"}, true, false, nil, nil)
	c.Check(err, gc.ErrorMatches, "this juju controller does not support non-destructive removal of storage")
}

func (s *storageMockSuite) TestRemoveInvalidStorageId(c *gc.C) {
	client := storage.NewClient(basetesting.APICallerFunc(
		func(_ string, _ int, _, _ string, _, _ interface{}) error {
			return nil
		},
	))
	_, err := client.Remove([]string{"foo/bar"}, false, false, nil, nil)
	c.Check(err, gc.ErrorMatches, `storage ID "foo/bar" not valid`)
}

func (s *storageMockSuite) assertDetach(c *gc.C, v int, expectedMethod string, expectedArgs interface{}) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Storage")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, expectedMethod)
				c.Check(a, jc.DeepEquals, expectedArgs)
				c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
				results := result.(*params.ErrorResults)
				results.Results = []params.ErrorResult{
					{},
					{Error: &params.Error{Message: "baz"}},
				}
				return nil
			},
		),
		BestVersion: v,
	}
	client := storage.NewClient(apiCaller)
	results, err := client.Detach([]string{"foo/0", "bar/1"}, nil, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results[0].Error, gc.IsNil)
	c.Assert(results[1].Error, jc.DeepEquals, &params.Error{Message: "baz"})
}

func (s *storageMockSuite) TestDetachV5(c *gc.C) {
	s.assertDetach(c, 5, "Detach",
		params.StorageAttachmentIds{[]params.StorageAttachmentId{
			{StorageTag: "storage-foo-0"},
			{StorageTag: "storage-bar-1"},
		}},
	)
}

func (s *storageMockSuite) TestDetach(c *gc.C) {
	s.assertDetach(c, 6, "DetachStorage",
		params.StorageDetachmentParams{
			StorageIds: params.StorageAttachmentIds{[]params.StorageAttachmentId{
				{StorageTag: "storage-foo-0"},
				{StorageTag: "storage-bar-1"},
			}}},
	)
}

func (s *storageMockSuite) TestDetachArityMismatch(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			results := result.(*params.ErrorResults)
			results.Results = []params.ErrorResult{{}, {}, {}}
			return nil
		},
	)
	client := storage.NewClient(apiCaller)
	_, err := client.Detach([]string{"foo/0", "bar/1"}, nil, nil)
	c.Check(err, gc.ErrorMatches, `expected 2 result\(s\), got 3`)
}

func (s *storageMockSuite) TestAttach(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Attach")
			c.Check(a, jc.DeepEquals, params.StorageAttachmentIds{[]params.StorageAttachmentId{
				{
					StorageTag: "storage-bar-1",
					UnitTag:    "unit-foo-0",
				},
				{
					StorageTag: "storage-baz-2",
					UnitTag:    "unit-foo-0",
				},
			}})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			results := result.(*params.ErrorResults)
			results.Results = []params.ErrorResult{
				{},
				{Error: &params.Error{Message: "qux"}},
			}
			return nil
		},
	)
	client := storage.NewClient(apiCaller)
	results, err := client.Attach("foo/0", []string{"bar/1", "baz/2"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results[0].Error, gc.IsNil)
	c.Assert(results[1].Error, jc.DeepEquals, &params.Error{Message: "qux"})
}

func (s *storageMockSuite) TestAttachArityMismatch(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			results := result.(*params.ErrorResults)
			results.Results = []params.ErrorResult{{}, {}, {}}
			return nil
		},
	)
	client := storage.NewClient(apiCaller)
	_, err := client.Attach("foo/0", []string{"bar/1", "baz/2"})
	c.Check(err, gc.ErrorMatches, `expected 2 result\(s\), got 3`)
}

func (s *storageMockSuite) TestImport(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Import")
			c.Check(a, jc.DeepEquals, params.BulkImportStorageParams{[]params.ImportStorageParams{{
				Kind:        params.StorageKindBlock,
				Pool:        "foo",
				ProviderId:  "bar",
				StorageName: "baz",
			}}})
			c.Assert(result, gc.FitsTypeOf, &params.ImportStorageResults{})
			results := result.(*params.ImportStorageResults)
			results.Results = []params.ImportStorageResult{{
				Result: &params.ImportStorageDetails{
					StorageTag: "storage-qux-0",
				},
			}}
			return nil
		},
	)
	client := storage.NewClient(apiCaller)
	storageTag, err := client.Import(jujustorage.StorageKindBlock, "foo", "bar", "baz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageTag, gc.Equals, names.NewStorageTag("qux/0"))
}

func (s *storageMockSuite) TestImportError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			results := result.(*params.ImportStorageResults)
			results.Results = []params.ImportStorageResult{{
				Error: &params.Error{Message: "qux"},
			}}
			return nil
		},
	)
	client := storage.NewClient(apiCaller)
	_, err := client.Import(jujustorage.StorageKindBlock, "foo", "bar", "baz")
	c.Check(err, gc.ErrorMatches, "qux")
}

func (s *storageMockSuite) TestImportArityMismatch(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			results := result.(*params.ImportStorageResults)
			results.Results = []params.ImportStorageResult{{}, {}}
			return nil
		},
	)
	client := storage.NewClient(apiCaller)
	_, err := client.Import(jujustorage.StorageKindBlock, "foo", "bar", "baz")
	c.Check(err, gc.ErrorMatches, `expected 1 result, got 2`)
}

func (s *storageMockSuite) TestRemovePool(c *gc.C) {
	var called bool
	poolName := "poolName"

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "RemovePool")

			args, ok := a.(params.StoragePoolDeleteArgs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Pools, gc.HasLen, 1)
			c.Assert(args.Pools[0].Name, gc.Equals, poolName)

			results := result.(*params.ErrorResults)
			results.Results = make([]params.ErrorResult, len(args.Pools))

			return nil
		})
	storageClient := storage.NewClient(basetesting.BestVersionCaller{BestVersion: 5, APICallerFunc: apiCaller})
	err := storageClient.RemovePool(poolName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestRemovePoolFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "RemovePool")

			args, ok := a.(params.StoragePoolDeleteArgs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Pools, gc.HasLen, 1)
			c.Assert(args.Pools[0].Name, gc.Equals, "")

			results := result.(*params.ErrorResults)
			results.Results = make([]params.ErrorResult, len(args.Pools))
			return errors.New(msg)
		})
	storageClient := storage.NewClient(basetesting.BestVersionCaller{BestVersion: 5, APICallerFunc: apiCaller})
	err := storageClient.RemovePool("")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *storageMockSuite) TestUpdatePool(c *gc.C) {
	var called bool
	poolName := "poolName"
	providerType := "loop"
	poolConfig := map[string]interface{}{
		"test": "one",
		"pass": true,
	}

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UpdatePool")

			args, ok := a.(params.StoragePoolArgs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Pools, gc.HasLen, 1)
			c.Assert(args.Pools[0].Name, gc.Equals, poolName)
			c.Assert(args.Pools[0].Provider, gc.Equals, providerType)
			c.Assert(args.Pools[0].Attrs, gc.DeepEquals, poolConfig)

			results := result.(*params.ErrorResults)
			results.Results = make([]params.ErrorResult, len(args.Pools))

			return nil
		})
	storageClient := storage.NewClient(basetesting.BestVersionCaller{BestVersion: 5, APICallerFunc: apiCaller})
	err := storageClient.UpdatePool(poolName, providerType, poolConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestUpdatePoolFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UpdatePool")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(basetesting.BestVersionCaller{BestVersion: 5, APICallerFunc: apiCaller})
	err := storageClient.UpdatePool("", "", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}
