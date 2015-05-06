// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type storageAddSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&storageAddSuite{})

func (s *storageAddSuite) assertStorageAddedNoErrors(c *gc.C, args params.StorageAddParams) {
	failures, err := s.api.Add(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(failures.Results, gc.HasLen, 0)
}

func (s *storageAddSuite) TestStorageAddEmpty(c *gc.C) {
	s.assertStorageAddedNoErrors(c, params.StorageAddParams{Storages: nil})
	s.assertStorageAddedNoErrors(c, params.StorageAddParams{Storages: []params.StorageDirective{}})
}

func (s *storageAddSuite) TestStorageAddUnit(c *gc.C) {
	args := params.StorageAddParams{
		UnitTag:  s.unitTag.String(),
		Storages: []params.StorageDirective{{Name: "data"}}}
	s.assertStorageAddedNoErrors(c, args)
	s.assertCalls(c, []string{getBlockForTypeCall, addStorageForUnitCall})
}

func (s *storageAddSuite) TestStorageAddUnitBlocked(c *gc.C) {
	s.blockAllChanges(c, "TestStorageAddUnitBlocked")

	args := params.StorageAddParams{
		UnitTag:  s.unitTag.String(),
		Storages: []params.StorageDirective{{Name: "data"}}}
	_, err := s.api.Add(args)
	s.assertBlocked(c, err, "TestStorageAddUnitBlocked")
}

func (s *storageAddSuite) TestStorageAddUnitDestroyIgnored(c *gc.C) {
	s.blockDestroyEnvironment(c, "TestStorageAddUnitDestroyIgnored")
	s.blockRemoveObject(c, "TestStorageAddUnitDestroyIgnored")

	args := params.StorageAddParams{
		UnitTag:  s.unitTag.String(),
		Storages: []params.StorageDirective{{Name: "data"}}}
	s.assertStorageAddedNoErrors(c, args)
	s.assertCalls(c, []string{getBlockForTypeCall, addStorageForUnitCall})
}

func (s *storageAddSuite) TestStorageAddUnitError(c *gc.C) {
	args := params.StorageAddParams{
		Storages: []params.StorageDirective{{Name: "data"}}}
	failures, err := s.api.Add(args)
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*is not a valid tag.*")
	c.Assert(failures.Results, gc.HasLen, 0)

	expectedCalls := []string{getBlockForTypeCall}
	s.assertCalls(c, expectedCalls)
}

func (s *storageAddSuite) TestStorageAddUnitDirectiveError(c *gc.C) {
	msg := "add test directive error"
	args := params.StorageAddParams{
		UnitTag:  s.unitTag.String(),
		Storages: []params.StorageDirective{{Name: "data"}}}
	s.state.addStorageForUnit = func(u names.UnitTag, name string, cons state.StorageConstraints) error {
		s.calls = append(s.calls, addStorageForUnitCall)
		return errors.Errorf(msg)
	}

	failures, err := s.api.Add(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(failures.Results, gc.HasLen, 1)
	c.Assert(failures.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(".*%v.*", msg))

	s.assertCalls(c, []string{getBlockForTypeCall, addStorageForUnitCall})
}
