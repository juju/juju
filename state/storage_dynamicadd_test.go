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

	err := s.State.AddStorage(ch.Meta(), u, "multi1to10", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, jc.ErrorIsNil)
	expectedStorages["multi1to10/5"] = true
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageWithCount(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorage(ch.Meta(), u, "multi1to10", makeStorageCons("loop-pool", 1024, 2))
	c.Assert(err, jc.ErrorIsNil)
	expectedStorages["multi1to10/5"] = true
	expectedStorages["multi1to10/6"] = true
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

	err = s.State.AddStorage(ch.Meta(), u, "data", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block" store "data": at most 1 instances supported, 2 specified.*`)
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageTriggerDefaultPopulated(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorage(ch.Meta(), u, "multi1to10", state.StorageConstraints{})
	c.Assert(err, jc.ErrorIsNil)
	expectedStorages["multi1to10/5"] = true
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageDiffPool(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorage(ch.Meta(), u, "multi1to10", state.StorageConstraints{Pool: "loop-pool"})
	c.Assert(err, jc.ErrorIsNil)
	expectedStorages["multi1to10/5"] = true
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageDiffSize(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorage(ch.Meta(), u, "multi1to10", state.StorageConstraints{Size: 2048})
	c.Assert(err, jc.ErrorIsNil)
	expectedStorages["multi1to10/5"] = true
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageLessMinSize(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorage(ch.Meta(), u, "multi2up", state.StorageConstraints{Size: 2})
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block2" store "multi2up": minimum storage size is 2.0GB, 2.0MB specified.*`)
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageWrongName(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)

	err := s.State.AddStorage(ch.Meta(), u, "furball", state.StorageConstraints{Size: 2})
	c.Assert(err, gc.ErrorMatches, `.*storage "furball" on the charm not found.*`)
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageConcurrently(c *gc.C) {
	ch, u, expectedStorages := s.setupMultipleStoragesForAdd(c)
	index := 4
	addStorage := func() {
		err := s.State.AddStorage(ch.Meta(), u, "multi1to10", state.StorageConstraints{})
		c.Assert(err, jc.ErrorIsNil)
		index++
		expectedStorages[fmt.Sprintf("multi1to10/%d", index)] = true
	}
	defer state.SetBeforeHooks(c, s.State, addStorage).Check()
	addStorage()

	c.Assert(expectedStorages, gc.HasLen, 7)
	s.assertMultiStorageExists(c, expectedStorages)
}

func (s *StorageStateSuite) TestAddStorageToService(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop", 0, 3),
	}
	ch := s.AddTestingCharm(c, "storage-block2")
	service, err := s.State.AddService("storage-block2", "user-test-admin@local", ch, nil, storageCons)
	c.Assert(err, jc.ErrorIsNil)

	// Get all storage before add.
	before, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.AddStorage(ch.Meta(), service, "multi1to10", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, jc.ErrorIsNil)

	// Get all storage afters add.
	after, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	// Can't add storage to service atm.
	c.Assert(len(before), gc.Equals, len(after))
}
