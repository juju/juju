// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

type storageAddSuite struct {
	baseStorageSuite
}

func TestStorageAddSuite(t *testing.T) {
	tc.Run(t, &storageAddSuite{})
}

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

	s.applicationService.EXPECT().AddStorageForUnit(
		gomock.Any(), corestorage.Name("data"), coreunit.Name(s.unitTag.Id()), storage.Directive{})

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	s.assertStorageAddedNoErrors(c, args)
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
	s.applicationService.EXPECT().AddStorageForUnit(
		gomock.Any(), corestorage.Name("data"), coreunit.Name(s.unitTag.Id()), storage.Directive{}).
		Return(nil, errors.New(msg))

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	failures, err := s.api.AddToUnit(c.Context(), params.StoragesAddParams{Storages: []params.StorageAddParams{args}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(failures.Results, tc.HasLen, 1)
	c.Assert(failures.Results[0].Error.Error(), tc.Matches, fmt.Sprintf(".*%v.*", msg))
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
	msg := "storage name missing"
	s.applicationService.EXPECT().AddStorageForUnit(
		gomock.Any(), corestorage.Name("data"), coreunit.Name(s.unitTag.Id()), storage.Directive{}).DoAndReturn(
		func(_ context.Context, name corestorage.Name, unit coreunit.Name, directive storage.Directive) ([]corestorage.ID, error) {
			if name == "" {
				return nil, errors.New(msg)
			}
			return nil, nil
		})
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
}

func (s *storageAddSuite) TestStorageAddUnitTags(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().AddStorageForUnit(
		gomock.Any(), corestorage.Name("data"), coreunit.Name(s.unitTag.Id()), storage.Directive{}).
		Return([]corestorage.ID{"foo/0", "foo/1"}, nil)

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

	s.applicationService.EXPECT().AddStorageForUnit(
		gomock.Any(), corestorage.Name("data"), coreunit.Name(s.unitTag.Id()), storage.Directive{}).
		Return(nil, storageerrors.StorageNotFound)

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	failures, err := s.api.AddToUnit(c.Context(), params.StoragesAddParams{[]params.StorageAddParams{args}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(failures.Results, tc.HasLen, 1)
	c.Assert(failures.Results[0].Error.Error(), tc.Matches, "storage data not found")
	c.Assert(failures.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *storageAddSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseStorageSuite.setupMocks(c)

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound).AnyTimes()

	return ctrl
}
