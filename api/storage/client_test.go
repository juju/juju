// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type storageMockSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageMockSuite{})

func (s *storageMockSuite) TestShow(c *gc.C) {
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
			c.Check(request, gc.Equals, "Show")

			args, ok := a.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Entities, gc.HasLen, 2)

			if results, k := result.(*params.StorageDetailsResults); k {
				instances := []params.StorageDetailsResult{
					params.StorageDetailsResult{
						Result: params.StorageDetails{StorageTag: oneTag.String()},
					},
					params.StorageDetailsResult{
						Result: params.StorageDetails{
							StorageTag: twoTag.String(),
							Status:     "attached",
							Persistent: true,
						},
					},
					params.StorageDetailsResult{Error: common.ServerError(errors.New(msg))},
				}
				results.Results = instances
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	tags := []names.StorageTag{oneTag, twoTag}
	found, err := storageClient.Show(tags)
	c.Check(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 2)
	c.Assert(expected.Contains(found[0].StorageTag), jc.IsTrue)
	c.Assert(expected.Contains(found[1].StorageTag), jc.IsTrue)
}

func (s *storageMockSuite) TestShowFacadeCallError(c *gc.C) {
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
			c.Check(request, gc.Equals, "Show")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.Show([]names.StorageTag{oneTag})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
}

func (s *storageMockSuite) TestList(c *gc.C) {
	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)
	two := "db-dir/1000"
	twoTag := names.NewStorageTag(two)
	msg := "call failure"

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "List")
			c.Check(a, gc.IsNil)

			if results, k := result.(*params.StorageInfosResult); k {
				instances := []params.StorageInfo{
					params.StorageInfo{
						params.StorageDetails{StorageTag: oneTag.String()},
						common.ServerError(errors.New(msg)),
					},
					params.StorageInfo{
						params.StorageDetails{
							StorageTag: twoTag.String(),
							Status:     "attached",
							Persistent: true,
						},
						nil,
					},
				}
				results.Results = instances
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.List()
	c.Check(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 2)
	expected := []params.StorageInfo{
		params.StorageInfo{
			StorageDetails: params.StorageDetails{
				StorageTag: "storage-shared-fs-0"},
			Error: &params.Error{Message: msg},
		},
		params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1000",
				Status:     "attached",
				Persistent: true},
			nil},
	}

	c.Assert(found, jc.DeepEquals, expected)
}

func (s *storageMockSuite) TestListFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "List")

			return errors.New(msg)
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.List()
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
}

func (s *storageMockSuite) TestListPools(c *gc.C) {
	expected := []params.StoragePool{
		params.StoragePool{Name: "name0", Provider: "type0"},
		params.StoragePool{Name: "name1", Provider: "type1"},
		params.StoragePool{Name: "name2", Provider: "type2"},
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

			args, ok := a.(params.StoragePoolFilter)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Names, gc.HasLen, 2)
			c.Assert(args.Providers, gc.HasLen, 1)

			if results, k := result.(*params.StoragePoolsResult); k {
				instances := make([]params.StoragePool, want)
				for i := 0; i < want; i++ {
					instances[i] = params.StoragePool{
						Name:     fmt.Sprintf("name%v", i),
						Provider: fmt.Sprintf("type%v", i),
					}
				}
				results.Results = instances
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	names := []string{"a", "b"}
	types := []string{"1"}
	found, err := storageClient.ListPools(types, names)
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

func (s *storageMockSuite) TestListVolumes(c *gc.C) {
	var called bool
	machines := []string{"one", "two"}
	machineTags := set.NewStrings(
		names.NewMachineTag(machines[0]).String(),
		names.NewMachineTag(machines[1]).String(),
	)
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

			c.Assert(a, gc.FitsTypeOf, params.VolumeFilter{})
			args := a.(params.VolumeFilter)
			c.Assert(args.Machines, gc.HasLen, 2)

			c.Assert(result, gc.FitsTypeOf, &params.VolumeItemsResult{})
			results := result.(*params.VolumeItemsResult)
			attachments := make([]params.VolumeAttachment, len(args.Machines))
			for i, m := range args.Machines {
				attachments[i] = params.VolumeAttachment{
					MachineTag: m}
			}
			results.Results = []params.VolumeItem{
				params.VolumeItem{Attachments: attachments},
			}
			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.ListVolumes(machines)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0].Attachments, gc.HasLen, len(machines))
	c.Assert(machineTags.Contains(found[0].Attachments[0].MachineTag), jc.IsTrue)
	c.Assert(machineTags.Contains(found[0].Attachments[1].MachineTag), jc.IsTrue)
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

			c.Assert(a, gc.FitsTypeOf, params.VolumeFilter{})
			args := a.(params.VolumeFilter)
			c.Assert(args.IsEmpty(), jc.IsTrue)

			c.Assert(result, gc.FitsTypeOf, &params.VolumeItemsResult{})
			results := result.(*params.VolumeItemsResult)
			results.Results = []params.VolumeItem{
				{Volume: params.VolumeInstance{VolumeTag: tag}},
			}
			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	found, err := storageClient.ListVolumes(nil)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0].Volume.VolumeTag, gc.Equals, tag)
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

func (s *storageMockSuite) TestAddToUnit(c *gc.C) {
	size := uint64(42)
	cons := params.StorageConstraints{
		Pool: "value",
		Size: &size,
	}

	errOut := "error"
	unitStorages := []params.StorageAddParams{
		params.StorageAddParams{UnitTag: "u-a", StorageName: "one", Constraints: cons},
		params.StorageAddParams{UnitTag: "u-b", StorageName: errOut, Constraints: cons},
		params.StorageAddParams{UnitTag: "u-b", StorageName: "nil-constraints"},
	}

	storageN := 3
	expectedError := common.ServerError(errors.NotValidf("storage directive"))
	one := func(u, s string, attrs params.StorageConstraints) params.ErrorResult {
		result := params.ErrorResult{}
		if s == errOut {
			result.Error = expectedError
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

			if results, k := result.(*params.ErrorResults); k {
				out := []params.ErrorResult{}
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
	expected := []params.ErrorResult{
		{nil},
		{expectedError},
		{nil},
	}
	c.Assert(r, jc.SameContents, expected)
}

func (s *storageMockSuite) TestAddToUnitFacadeCallError(c *gc.C) {
	unitStorages := []params.StorageAddParams{
		params.StorageAddParams{UnitTag: "u-a", StorageName: "one"},
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
