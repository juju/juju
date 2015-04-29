// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

func (s *StorageStateSuite) setupMultipleStoragesForAdd(c *gc.C) (*state.Charm, *state.Unit, int) {
	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop", 0, 3),
	}
	ch := s.AddTestingCharm(c, "storage-block2")
	service, err := s.State.AddService("storage-block2", "user-test-admin@local", ch, nil, storageCons)
	c.Assert(err, jc.ErrorIsNil)
	u, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	original, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	return ch, u, len(original)
}

func (s *StorageStateSuite) assertStorageAddedDynamically(c *gc.C, original, expected int) {
	final, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(final)-original, gc.Equals, expected)
}

func (s *StorageStateSuite) TestAddStorageToUnit(c *gc.C) {
	ch, u, original := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageAddedDynamically(c, original, 1)
}

func (s *StorageStateSuite) TestAddStorageWithCount(c *gc.C) {
	ch, u, original := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", makeStorageCons("loop-pool", 1024, 2))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageAddedDynamically(c, original, 2)
}

func (s *StorageStateSuite) TestAddStorageMultipleCalls(c *gc.C) {
	ch, u, original := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", makeStorageCons("loop-pool", 1024, 2))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageAddedDynamically(c, original, 2)

	// Should not succeed as the number of storages after
	// this call would be 11 whereas our upper limit is 10 here.
	err = s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", makeStorageCons("loop-pool", 1024, 6))
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block2" store "multi1to10": at most 10 instances supported, 11 specified.*`)
	s.assertStorageAddedDynamically(c, original+2, 0)
}

func (s *StorageStateSuite) TestAddStorageExceedCount(c *gc.C) {
	service, u, _ := s.setupSingleStorage(c, "block", "loop-pool")
	s.assertStorageAddedDynamically(c, 0, 1)

	ch, _, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.AddStorageForUnit(ch.Meta(), u, "data", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block" store "data": at most 1 instances supported, 2 specified.*`)
	s.assertStorageAddedDynamically(c, 1, 0)
}

func (s *StorageStateSuite) TestAddStorageTriggerDefaultPopulated(c *gc.C) {
	ch, u, original := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", state.StorageConstraints{})
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageAddedDynamically(c, original, 1)
}

func (s *StorageStateSuite) TestAddStorageDiffPool(c *gc.C) {
	ch, u, original := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", state.StorageConstraints{Pool: "loop-pool"})
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageAddedDynamically(c, original, 1)
}

func (s *StorageStateSuite) TestAddStorageDiffSize(c *gc.C) {
	ch, u, original := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", state.StorageConstraints{Size: 2048})
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageAddedDynamically(c, original, 1)
}

func (s *StorageStateSuite) TestAddStorageLessMinSize(c *gc.C) {
	ch, u, original := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi2up", state.StorageConstraints{Size: 2})
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block2" store "multi2up": minimum storage size is 2.0GB, 2.0MB specified.*`)
	s.assertStorageAddedDynamically(c, original, 0)
}

func (s *StorageStateSuite) TestAddStorageWrongName(c *gc.C) {
	ch, u, original := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "furball", state.StorageConstraints{Size: 2})
	c.Assert(err, gc.ErrorMatches, `.*charm storage "furball" not found.*`)
	s.assertStorageAddedDynamically(c, original, 0)
}

func (s *StorageStateSuite) TestAddStorageConcurrently(c *gc.C) {
	ch, u, original := s.setupMultipleStoragesForAdd(c)
	addStorage := func() {
		s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", state.StorageConstraints{})
	}
	defer state.SetBeforeHooks(c, s.State, addStorage).Check()
	addStorage()
	// Only 1 should have been added
	s.assertStorageAddedDynamically(c, original, 1)
}

func (s *StorageStateSuite) TestAddStorageConcurrentlyExceedCount(c *gc.C) {
	ch, u, original := s.setupMultipleStoragesForAdd(c)

	count := 6
	addStorage := func() {
		s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", state.StorageConstraints{Count: uint64(count)})
	}
	defer state.SetBeforeHooks(c, s.State, addStorage).Check()
	addStorage()
	// Only "count" number of instances should have been added.
	s.assertStorageAddedDynamically(c, original, count)
}
