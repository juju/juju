// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	blockcommand "github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type storageAddSuite struct {
	baseStorageSuite
}

var _ = tc.Suite(&storageAddSuite{})

func (s *storageAddSuite) assertStorageAddedNoErrors(c *tc.C, args params.StorageAddParams) {
	s.assertStoragesAddedNoErrors(c,
		params.StoragesAddParams{Storages: []params.StorageAddParams{args}},
	)
}

func (s *storageAddSuite) assertStoragesAddedNoErrors(c *tc.C, args params.StoragesAddParams) {
	failures, err := s.api.AddToUnit(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(failures.Results, tc.HasLen, len(args.Storages))
	for _, one := range failures.Results {
		c.Assert(one.Error, tc.IsNil)
	}
}

func (s *storageAddSuite) TestStorageAddEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.assertStoragesAddedNoErrors(c, params.StoragesAddParams{Storages: nil})
	s.assertStoragesAddedNoErrors(c, params.StoragesAddParams{Storages: []params.StorageAddParams{}})
}

func (s *storageAddSuite) TestStorageAddUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	s.assertStorageAddedNoErrors(c, args)
	s.assertCalls(c, []string{addStorageForUnitCall})
}

func (s *storageAddSuite) TestStorageAddUnitBlocked(c *tc.C) {
	defer s.baseStorageSuite.setupMocks(c).Finish()

	s.blockAllChanges(c, "TestStorageAddUnitBlocked")

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	_, err := s.api.AddToUnit(c.Context(), params.StoragesAddParams{Storages: []params.StorageAddParams{args}})
	s.assertBlocked(c, err, "TestStorageAddUnitBlocked")
}

func (s *storageAddSuite) TestStorageAddUnitDestroyIgnored(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	s.assertStorageAddedNoErrors(c, args)
	s.assertCalls(c, []string{addStorageForUnitCall})
}

func (s *storageAddSuite) TestStorageAddUnitInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := params.StorageAddParams{
		UnitTag:     "invalid-unit-name",
		StorageName: "data",
	}
	failures, err := s.api.AddToUnit(c.Context(), params.StoragesAddParams{Storages: []params.StorageAddParams{args}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(failures.Results, tc.HasLen, 1)
	c.Assert(failures.Results[0].Error.Error(), tc.Matches, "\"invalid-unit-name\" is not a valid tag")

	expectedCalls := []string{}
	s.assertCalls(c, expectedCalls)
}

func (s *storageAddSuite) TestStorageAddUnitStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	msg := "add test directive error"
	s.storageAccessor.addStorageForUnit = func(u names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error) {
		s.stub.AddCall(addStorageForUnitCall)
		return nil, errors.New(msg)
	}

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	failures, err := s.api.AddToUnit(c.Context(), params.StoragesAddParams{Storages: []params.StorageAddParams{args}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(failures.Results, tc.HasLen, 1)
	c.Assert(failures.Results[0].Error.Error(), tc.Matches, fmt.Sprintf(".*%v.*", msg))

	s.assertCalls(c, []string{addStorageForUnitCall})
}

func (s *storageAddSuite) TestStorageAddUnitResultOrder(c *tc.C) {
	defer s.setupMocks(c).Finish()

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
	failures, err := s.api.AddToUnit(c.Context(), params.StoragesAddParams{
		Storages: []params.StorageAddParams{
			wrong0,
			right,
			wrong1,
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(failures.Results, tc.HasLen, 3)
	c.Assert(failures.Results[0].Error.Error(), tc.Matches, ".*is not a valid tag.*")
	c.Assert(failures.Results[1].Error, tc.IsNil)
	c.Assert(failures.Results[2].Error.Error(), tc.Matches, fmt.Sprintf(".*%v.*", msg))

	s.assertCalls(c, []string{addStorageForUnitCall, addStorageForUnitCall})
}

func (s *storageAddSuite) TestStorageAddUnitTags(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tags := []names.StorageTag{names.NewStorageTag("foo/0"), names.NewStorageTag("foo/1")}
	s.storageAccessor.addStorageForUnit = func(u names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error) {
		return tags, nil
	}

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	results, err := s.api.AddToUnit(c.Context(), params.StoragesAddParams{[]params.StorageAddParams{args}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.DeepEquals, []params.AddStorageResult{{
		Result: &params.AddStorageDetails{
			StorageTags: []string{"storage-foo-0", "storage-foo-1"},
		},
	}})
}

func (s *storageAddSuite) TestStorageAddUnitNotFoundErr(c *tc.C) {
	defer s.setupMocks(c).Finish()

	msg := "sanity"
	s.storageAccessor.addStorageForUnit = func(u names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error) {
		s.stub.AddCall(addStorageForUnitCall)
		return nil, errors.NotFoundf(msg)
	}

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	failures, err := s.api.AddToUnit(c.Context(), params.StoragesAddParams{[]params.StorageAddParams{args}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(failures.Results, tc.HasLen, 1)
	c.Assert(failures.Results[0].Error.Error(), tc.Matches, "sanity not found")
	c.Assert(failures.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *storageAddSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseStorageSuite.setupMocks(c)

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound).AnyTimes()

	return ctrl
}
