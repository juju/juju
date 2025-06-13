// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type storageAddSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&storageAddSuite{})

func (s *storageAddSuite) assertStorageAddedNoErrors(c *gc.C, args params.StorageAddParams) {
	s.assertStoragesAddedNoErrors(c,
		params.StoragesAddParams{[]params.StorageAddParams{args}},
	)
}

func (s *storageAddSuite) assertStoragesAddedNoErrors(c *gc.C, args params.StoragesAddParams) {
	failures, err := s.api.AddToUnit(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(failures.Results, gc.HasLen, len(args.Storages))
	for _, one := range failures.Results {
		c.Assert(one.Error, gc.IsNil)
	}
}

func (s *storageAddSuite) TestStorageAddEmpty(c *gc.C) {
	s.assertStoragesAddedNoErrors(c, params.StoragesAddParams{Storages: nil})
	s.assertStoragesAddedNoErrors(c, params.StoragesAddParams{Storages: []params.StorageAddParams{}})
}

func (s *storageAddSuite) TestStorageAddUnit(c *gc.C) {
	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	s.assertStorageAddedNoErrors(c, args)
	s.assertCalls(c, []string{getBlockForTypeCall, addStorageForUnitCall})
}

func (s *storageAddSuite) TestStorageAddUnitBlocked(c *gc.C) {
	s.blockAllChanges(c, "TestStorageAddUnitBlocked")

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	_, err := s.api.AddToUnit(params.StoragesAddParams{[]params.StorageAddParams{args}})
	s.assertBlocked(c, err, "TestStorageAddUnitBlocked")
}

func (s *storageAddSuite) TestStorageAddUnitDestroyIgnored(c *gc.C) {
	s.blockDestroyModel(c, "TestStorageAddUnitDestroyIgnored")
	s.blockRemoveObject(c, "TestStorageAddUnitDestroyIgnored")

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	s.assertStorageAddedNoErrors(c, args)
	s.assertCalls(c, []string{getBlockForTypeCall, addStorageForUnitCall})
}

func (s *storageAddSuite) TestStorageAddUnitInvalidName(c *gc.C) {
	args := params.StorageAddParams{
		UnitTag:     "invalid-unit-name",
		StorageName: "data",
	}
	failures, err := s.api.AddToUnit(params.StoragesAddParams{[]params.StorageAddParams{args}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(failures.Results, gc.HasLen, 1)
	c.Assert(failures.Results[0].Error.Error(), gc.Matches, "\"invalid-unit-name\" is not a valid tag")

	expectedCalls := []string{getBlockForTypeCall}
	s.assertCalls(c, expectedCalls)
}

func (s *storageAddSuite) TestStorageAddUnitStateError(c *gc.C) {
	msg := "add test directive error"
	s.storageAccessor.addStorageForUnit = func(u names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error) {
		s.stub.AddCall(addStorageForUnitCall)
		return nil, errors.New(msg)
	}

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	failures, err := s.api.AddToUnit(params.StoragesAddParams{[]params.StorageAddParams{args}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(failures.Results, gc.HasLen, 1)
	c.Assert(failures.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(".*%v.*", msg))

	s.assertCalls(c, []string{getBlockForTypeCall, addStorageForUnitCall})
}

func (s *storageAddSuite) TestStorageAddUnitResultOrder(c *gc.C) {
	wrong0 := params.StorageAddParams{
		StorageName: "data",
	}
	right := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	wrong1 := params.StorageAddParams{
		UnitTag: s.unitTag.String(),
	}
	msg := "storage name missing error"
	s.storageAccessor.addStorageForUnit = func(u names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error) {
		s.stub.AddCall(addStorageForUnitCall)
		if name == "" {
			return nil, errors.New(msg)
		}
		return nil, nil
	}
	failures, err := s.api.AddToUnit(params.StoragesAddParams{
		[]params.StorageAddParams{
			wrong0,
			right,
			wrong1,
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(failures.Results, gc.HasLen, 3)
	c.Assert(failures.Results[0].Error.Error(), gc.Matches, ".*is not a valid tag.*")
	c.Assert(failures.Results[1].Error, gc.IsNil)
	c.Assert(failures.Results[2].Error.Error(), gc.Matches, fmt.Sprintf(".*%v.*", msg))

	s.assertCalls(c, []string{getBlockForTypeCall, addStorageForUnitCall, addStorageForUnitCall})
}

func (s *storageAddSuite) TestStorageAddUnitTags(c *gc.C) {
	tags := []names.StorageTag{names.NewStorageTag("foo/0"), names.NewStorageTag("foo/1")}
	s.storageAccessor.addStorageForUnit = func(u names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error) {
		return tags, nil
	}

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	results, err := s.api.AddToUnit(params.StoragesAddParams{[]params.StorageAddParams{args}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.AddStorageResult{{
		Result: &params.AddStorageDetails{
			StorageTags: []string{"storage-foo-0", "storage-foo-1"},
		},
	}})
}

func (s *storageAddSuite) TestStorageAddUnitNotFoundErr(c *gc.C) {
	msg := "sanity"
	s.storageAccessor.addStorageForUnit = func(u names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error) {
		s.stub.AddCall(addStorageForUnitCall)
		return nil, errors.NotFoundf(msg)
	}

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	failures, err := s.api.AddToUnit(params.StoragesAddParams{[]params.StorageAddParams{args}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(failures.Results, gc.HasLen, 1)
	c.Assert(failures.Results[0].Error.Error(), gc.Matches, "sanity not found")
	c.Assert(failures.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}
