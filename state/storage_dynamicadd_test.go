// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

func (s *StorageStateSuite) setupMultipleStoragesForAdd(c *gc.C) (*state.Charm, *state.Unit, map[string]bool) {
	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop", 0, 3),
	}
	ch := s.AddTestingCharm(c, "storage-block2")
	service, err := s.State.AddService("storage-block2", "user-test-admin@local", ch, nil, storageCons)
	c.Assert(err, jc.ErrorIsNil)
	u, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	expectedStorages := map[string]bool{
		"multi1to10/0": true,
		"multi1to10/1": true,
		"multi1to10/2": true,
		"multi1to10/5": false, // will be created dynamically during this test
		"multi2up/3":   true,
		"multi2up/4":   true,
	}
	s.assertMultiStorageExists(c, expectedStorages)
	return ch, u, expectedStorages
}

func (s *StorageStateSuite) TestAddStorageToUnit(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, jc.ErrorIsNil)
	expectedStorages["multi1to10/5"] = true
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageWithCount(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", makeStorageCons("loop-pool", 1024, 2))
	c.Assert(err, jc.ErrorIsNil)
	expectedStorages["multi1to10/5"] = true
	expectedStorages["multi1to10/6"] = true
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageMultipleCalls(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", makeStorageCons("loop-pool", 1024, 2))
	c.Assert(err, jc.ErrorIsNil)
	expectedStorages["multi1to10/5"] = true
	expectedStorages["multi1to10/6"] = true
	s.assertMultiStorageExists(c, expectedStorages)

	// Should not succeed as the number of storages after
	// this call would be 11 whereas our upper limit is 10 here.
	err = s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", makeStorageCons("loop-pool", 1024, 6))
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block2" store "multi1to10": at most 10 instances supported, 11 specified.*`)
	expectedStorages["multi1to10/7"] = false
	expectedStorages["multi1to10/8"] = false
	expectedStorages["multi1to10/9"] = false
	expectedStorages["multi1to10/10"] = false
	expectedStorages["multi1to10/11"] = false
	expectedStorages["multi1to10/12"] = false
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) assertMultiStorageExists(c *gc.C, all map[string]bool) {
	for tag, exists := range all {
		confirm := s.storageInstanceExists(c, names.NewStorageTag(tag))
		c.Assert(exists, gc.Equals, confirm)
	}
}

func (s *StorageStateSuite) TestAddStorageExceedCount(c *gc.C) {
	service, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	expectedStorages := map[string]bool{
		storageTag.Id(): true,
		"data/1":        false, // will try to create in this test
	}
	s.assertMultiStorageExists(c, expectedStorages)

	ch, _, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.AddStorageForUnit(ch.Meta(), u, "data", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block" store "data": at most 1 instances supported, 2 specified.*`)
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageTriggerDefaultPopulated(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", state.StorageConstraints{})
	c.Assert(err, jc.ErrorIsNil)
	expectedStorages["multi1to10/5"] = true
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageDiffPool(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", state.StorageConstraints{Pool: "loop-pool"})
	c.Assert(err, jc.ErrorIsNil)
	expectedStorages["multi1to10/5"] = true
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageDiffSize(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", state.StorageConstraints{Size: 2048})
	c.Assert(err, jc.ErrorIsNil)
	expectedStorages["multi1to10/5"] = true
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageLessMinSize(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "multi2up", state.StorageConstraints{Size: 2})
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block2" store "multi2up": minimum storage size is 2.0GB, 2.0MB specified.*`)
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageWrongName(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorageForUnit(ch.Meta(), u, "furball", state.StorageConstraints{Size: 2})
	c.Assert(err, gc.ErrorMatches, `.*charm storage "furball" not found.*`)
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageConcurrently(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)
	index := 4
	addStorage := func() {
		err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", state.StorageConstraints{})
		c.Assert(err, jc.ErrorIsNil)
		index++
		expectedStorages[fmt.Sprintf("multi1to10/%d", index)] = true
	}
	defer state.SetBeforeHooks(c, s.State, addStorage).Check()
	addStorage()

	c.Assert(expectedStorages, gc.HasLen, 7)
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageConcurrentlyExceedCount(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)
	original, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)

	index := 4
	count := 6
	addStorage := func() {
		err := s.State.AddStorageForUnit(ch.Meta(), u, "multi1to10", state.StorageConstraints{Count: uint64(count)})
		c.Assert(err, jc.ErrorIsNil)
		for i := 0; i < count; i++ {
			index++
			expectedStorages[fmt.Sprintf("multi1to10/%d", index)] = true
		}
	}
	defer state.SetBeforeHooks(c, s.State, addStorage).Check()
	addStorage()

	final, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(len(final)-len(original), gc.Equals, count)
	s.assertMultiStorageExists(c, expectedStorages)
}
