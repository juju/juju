// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storage"
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
	s.blockDestroyEnvironment(c, "TestStorageAddUnitDestroyIgnored")
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
	c.Assert(failures.Results[0].Error.Error(), gc.Matches, "parsing unit tag invalid-unit-name: \"invalid-unit-name\" is not a valid tag")

	expectedCalls := []string{getBlockForTypeCall}
	s.assertCalls(c, expectedCalls)
}

func (s *storageAddSuite) TestStorageAddUnitStateError(c *gc.C) {
	msg := "add test directive error"
	s.state.addStorageForUnit = func(u names.UnitTag, name string, cons state.StorageConstraints) error {
		s.calls = append(s.calls, addStorageForUnitCall)
		return errors.Errorf(msg)
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
	s.state.addStorageForUnit = func(u names.UnitTag, name string, cons state.StorageConstraints) error {
		s.calls = append(s.calls, addStorageForUnitCall)
		if name == "" {
			return errors.Errorf(msg)
		}
		return nil
	}
	failures, err := s.api.AddToUnit(params.StoragesAddParams{
		[]params.StorageAddParams{
			wrong0,
			right,
			wrong1}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(failures.Results, gc.HasLen, 3)
	c.Assert(failures.Results[0].Error.Error(), gc.Matches, ".*is not a valid tag.*")
	c.Assert(failures.Results[1].Error, gc.IsNil)
	c.Assert(failures.Results[2].Error.Error(), gc.Matches, fmt.Sprintf(".*%v.*", msg))

	s.assertCalls(c, []string{getBlockForTypeCall, addStorageForUnitCall, addStorageForUnitCall})
}

func (s *storageAddSuite) TestStorageAddUnitIsAdminError(c *gc.C) {
	msg := "cannot determine isControllerAdministrator error"
	s.state.isControllerAdministrator = func(user names.UserTag) (bool, error) {
		s.calls = append(s.calls, "isControllerAdministrator")
		return false, errors.New(msg)
	}

	_, err := storage.CreateAPI(s.state, s.poolManager, s.resources, s.authorizer)
	c.Assert(err, gc.ErrorMatches, msg)
	s.assertCalls(c, []string{"isControllerAdministrator"})
}

func (s *storageAddSuite) TestStorageAddUnitAdminCanSeeNotFoundErr(c *gc.C) {
	s.assertAddUnitReturnedError(c, "adding storage data for unit-mysql-0: add test directive error not found")
}

func (s *storageAddSuite) TestStorageAddUnitNonAdminCannotSeeNotFoundErr(c *gc.C) {
	s.state.isControllerAdministrator = func(user names.UserTag) (bool, error) {
		return false, nil
	}

	var err error
	s.api, err = storage.CreateAPI(s.state, s.poolManager, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAddUnitReturnedError(c, ".*permission denied.*")
}

func (s *storageAddSuite) assertAddUnitReturnedError(c *gc.C, expectedErr string) {
	msg := "add test directive error"
	s.state.addStorageForUnit = func(u names.UnitTag, name string, cons state.StorageConstraints) error {
		s.calls = append(s.calls, addStorageForUnitCall)
		return errors.NotFoundf(msg)
	}

	args := params.StorageAddParams{
		UnitTag:     s.unitTag.String(),
		StorageName: "data",
	}
	failures, err := s.api.AddToUnit(params.StoragesAddParams{[]params.StorageAddParams{args}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(failures.Results, gc.HasLen, 1)
	c.Assert(failures.Results[0].Error.Error(), gc.Matches, expectedErr)

	s.assertCalls(c, []string{getBlockForTypeCall, addStorageForUnitCall})
}
