// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type StorageAddSuite struct {
	StorageStateSuiteBase

	unitTag              names.UnitTag
	originalStorageCount int
}

var _ = gc.Suite(&StorageAddSuite{})

func (s *StorageAddSuite) setupMultipleStoragesForAdd(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop", 0, 3),
	}
	charm := s.AddTestingCharm(c, "storage-block2")
	service, err := s.State.AddService("storage-block2", "user-test-admin@local", charm, nil, storageCons)
	c.Assert(err, jc.ErrorIsNil)
	u, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	s.unitTag = u.Tag().(names.UnitTag)

	all, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	s.originalStorageCount = len(all)
}

func (s *StorageAddSuite) assertStorageCount(c *gc.C, expected int) {
	all, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(all), gc.Equals, expected)
}

func (s *StorageAddSuite) TestAddStorageToUnit(c *gc.C) {
	s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(s.unitTag, "multi1to10", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, s.originalStorageCount+1)
}

func (s *StorageAddSuite) TestAddStorageWithCount(c *gc.C) {
	s.setupMultipleStoragesForAdd(c)
	err := s.State.AddStorageForUnit(s.unitTag, "multi1to10", makeStorageCons("loop-pool", 1024, 2))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, s.originalStorageCount+2)
}

func (s *StorageAddSuite) TestAddStorageMultipleCalls(c *gc.C) {
	s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(s.unitTag, "multi1to10", makeStorageCons("loop-pool", 1024, 2))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, s.originalStorageCount+2)

	// Should not succeed as the number of storages after
	// this call would be 11 whereas our upper limit is 10 here.
	err = s.State.AddStorageForUnit(s.unitTag, "multi1to10", makeStorageCons("loop-pool", 1024, 6))
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block2" store "multi1to10": at most 10 instances supported, 11 specified.*`)
	s.assertStorageCount(c, s.originalStorageCount+2)
}

func (s *StorageAddSuite) TestAddStorageExceedCount(c *gc.C) {
	_, u, _ := s.setupSingleStorage(c, "block", "loop-pool")
	s.assertStorageCount(c, 1)

	err := s.State.AddStorageForUnit(u.Tag().(names.UnitTag), "data", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block" store "data": at most 1 instances supported, 2 specified.*`)
	s.assertStorageCount(c, 1)
}

func (s *StorageAddSuite) TestAddStorageMinCount(c *gc.C) {
	_, u, _ := s.setupSingleStorage(c, "block", "loop-pool")
	s.assertStorageCount(c, 1)

	err := s.State.AddStorageForUnit(u.Tag().(names.UnitTag), "allecto", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, 2)
}

func (s *StorageAddSuite) TestAddStorageZeroCount(c *gc.C) {
	_, u, _ := s.setupSingleStorage(c, "block", "loop-pool")
	s.assertStorageCount(c, 1)

	err := s.State.AddStorageForUnit(u.Tag().(names.UnitTag), "allecto", state.StorageConstraints{Pool: "loop-pool", Size: 1024})
	c.Assert(errors.Cause(err), gc.ErrorMatches, "adding storage where instance count is 0 not valid")
	s.assertStorageCount(c, 1)
}

func (s *StorageAddSuite) TestAddStorageTriggerDefaultPopulated(c *gc.C) {
	s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Count: 1})
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, s.originalStorageCount+1)
}

func (s *StorageAddSuite) TestAddStorageDiffPool(c *gc.C) {
	s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Pool: "loop-pool", Count: 1})
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, s.originalStorageCount+1)
}

func (s *StorageAddSuite) TestAddStorageDiffSize(c *gc.C) {
	s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Size: 2048, Count: 1})
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, s.originalStorageCount+1)
}

func (s *StorageAddSuite) TestAddStorageLessMinSize(c *gc.C) {
	s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(s.unitTag, "multi2up", state.StorageConstraints{Size: 2, Count: 1})
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block2" store "multi2up": minimum storage size is 2.0GB, 2.0MB specified.*`)
	s.assertStorageCount(c, s.originalStorageCount)
}

func (s *StorageAddSuite) TestAddStorageWrongName(c *gc.C) {
	s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(s.unitTag, "furball", state.StorageConstraints{Size: 2})
	c.Assert(err, gc.ErrorMatches, `.*charm storage "furball" not found.*`)
	s.assertStorageCount(c, s.originalStorageCount)
}

func (s *StorageAddSuite) TestAddStorageConcurrently(c *gc.C) {
	s.setupMultipleStoragesForAdd(c)
	addStorage := func() {
		err := s.State.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Count: 1})
		c.Assert(err, jc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, addStorage).Check()
	addStorage()
	s.assertStorageCount(c, s.originalStorageCount+2)
}

func (s *StorageAddSuite) TestAddStorageConcurrentlyExceedCount(c *gc.C) {
	s.setupMultipleStoragesForAdd(c)

	count := 6
	addStorage := func() {
		err := s.State.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Count: uint64(count)})
		c.Assert(err, jc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, addStorage).Check()
	err := s.State.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Count: uint64(count)})
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block2" store "multi1to10": at most 10 instances supported, 15 specified.*`)

	// Only "count" number of instances should have been added.
	s.assertStorageCount(c, s.originalStorageCount+count)
}
