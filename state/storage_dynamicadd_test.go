// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type storageAddSuite struct {
	StorageStateSuiteBase

	unitTag    names.UnitTag
	machineTag names.MachineTag

	originalStorageCount    int
	originalVolumeCount     int
	originalFilesystemCount int
}

var _ = gc.Suite(&storageAddSuite{})

func (s *storageAddSuite) setupMultipleStoragesForAdd(c *gc.C) *state.Unit {
	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("persistent-block", 0, 3),
	}
	charm := s.AddTestingCharm(c, "storage-block2")
	application, err := s.State.AddApplication(state.AddApplicationArgs{Name: "storage-block2", Charm: charm, Storage: storageCons})
	c.Assert(err, jc.ErrorIsNil)
	u, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.unitTag = u.UnitTag()
	all, err := s.storageBackend.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	s.originalStorageCount = len(all)
	return u
}

func (s *storageAddSuite) assignUnit(c *gc.C, u *state.Unit) {
	// Assign unit to machine to get volumes and filesystems
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	machineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	s.machineTag = m.MachineTag()

	volumes, err := s.storageBackend.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	s.originalVolumeCount = len(volumes)

	filesystems, err := s.storageBackend.MachineFilesystemAttachments(s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.originalFilesystemCount = len(filesystems)
}

func (s *storageAddSuite) assertStorageCount(c *gc.C, count int) {
	all, err := s.storageBackend.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, count)
}

func (s *storageAddSuite) assertVolumeCount(c *gc.C, count int) {
	all, err := s.storageBackend.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, count)
}

func (s *storageAddSuite) assertFileSystemCount(c *gc.C, count int) {
	all, err := s.storageBackend.MachineFilesystemAttachments(s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, count)
}

func (s *storageAddSuite) TestAddStorageToUnit(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	s.assignUnit(c, u)

	tags, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", makeStorageCons("loop-pool", 4096, 1))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tags, jc.DeepEquals, []names.StorageTag{
		names.NewStorageTag("multi1to10/5"),
	})

	s.assertStorageCount(c, s.originalStorageCount+1)
	s.assertVolumeCount(c, s.originalVolumeCount+1)
	s.assertFileSystemCount(c, s.originalFilesystemCount)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)

	allVolumeParams := allMachineVolumeParams(c, s.storageBackend, s.machineTag)
	c.Assert(allVolumeParams, jc.SameContents, []state.VolumeParams{
		{Pool: "persistent-block", Size: 1024}, // multi1to10
		{Pool: "persistent-block", Size: 1024}, // multi1to10
		{Pool: "persistent-block", Size: 1024}, // multi1to10
		{Pool: "loop", Size: 2048},             // multi2up
		{Pool: "loop", Size: 2048},             // multi2up
		{Pool: "loop-pool", Size: 4096},        // added above
	})
}

func (s *storageAddSuite) TestAddStorageToUnitInheritPoolAndSize(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	s.assignUnit(c, u)

	_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Count: 1})
	c.Assert(err, jc.ErrorIsNil)

	allVolumeParams := allMachineVolumeParams(c, s.storageBackend, s.machineTag)
	c.Assert(allVolumeParams, jc.SameContents, []state.VolumeParams{
		{Pool: "persistent-block", Size: 1024},
		{Pool: "persistent-block", Size: 1024},
		{Pool: "persistent-block", Size: 1024},
		{Pool: "persistent-block", Size: 1024},
		{Pool: "loop", Size: 2048},
		{Pool: "loop", Size: 2048},
	})
}

func (s *storageAddSuite) TestAddStorageToUnitNotAssigned(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	// don't assign unit

	_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", makeStorageCons("loop-pool", 4096, 1))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, s.originalStorageCount+1)
	s.assertVolumeCount(c, 0)
	s.assertFileSystemCount(c, 0)

	s.assignUnit(c, u)
	s.assertVolumeCount(c, 6)
	s.assertFileSystemCount(c, 0)

	allVolumeParams := allMachineVolumeParams(c, s.storageBackend, s.machineTag)
	c.Assert(allVolumeParams, jc.SameContents, []state.VolumeParams{
		{Pool: "persistent-block", Size: 1024},
		{Pool: "persistent-block", Size: 1024},
		{Pool: "persistent-block", Size: 1024},
		{Pool: "loop", Size: 2048},
		{Pool: "loop", Size: 2048},
		{Pool: "loop-pool", Size: 4096},
	})
}

func allMachineVolumeParams(c *gc.C, sb *state.StorageBackend, m names.MachineTag) []state.VolumeParams {
	var allVolumeParams []state.VolumeParams
	volumeAttachments, err := sb.MachineVolumeAttachments(m)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range volumeAttachments {
		volume, err := sb.Volume(a.Volume())
		c.Assert(err, jc.ErrorIsNil)
		volumeParams, ok := volume.Params()
		c.Assert(ok, jc.IsTrue)
		allVolumeParams = append(allVolumeParams, volumeParams)
	}
	return allVolumeParams
}

func (s *storageAddSuite) TestAddStorageWithCount(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	s.assignUnit(c, u)
	tags, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", makeStorageCons("loop-pool", 1024, 2))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tags, jc.DeepEquals, []names.StorageTag{
		names.NewStorageTag("multi1to10/5"),
		names.NewStorageTag("multi1to10/6"),
	})
	s.assertStorageCount(c, s.originalStorageCount+2)
	s.assertVolumeCount(c, s.originalVolumeCount+2)
	s.assertFileSystemCount(c, s.originalFilesystemCount)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}

func (s *storageAddSuite) TestAddStorageMultipleCalls(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	s.assignUnit(c, u)

	_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", makeStorageCons("loop-pool", 1024, 2))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, s.originalStorageCount+2)

	// Should not succeed as the number of storages after
	// this call would be 11 whereas our upper limit is 10 here.
	_, err = s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", makeStorageCons("loop-pool", 1024, 6))
	c.Assert(err, gc.ErrorMatches,
		`adding "multi1to10" storage to storage-block2/0: `+
			`attaching 6 storage instances brings the total to 11, exceeding the maximum of 10`)
	s.assertStorageCount(c, s.originalStorageCount+2)
	s.assertVolumeCount(c, s.originalVolumeCount+2)
	s.assertFileSystemCount(c, s.originalFilesystemCount)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}

func (s *storageAddSuite) TestAddStorageToDyingUnitFails(c *gc.C) {
	s.setupMultipleStoragesForAdd(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		u, err := s.State.Unit(s.unitTag.Id())
		c.Assert(err, jc.ErrorIsNil)
		err = u.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, gc.ErrorMatches, `adding "multi1to10" storage to storage-block2/0: unit is not found or not alive`)

	s.assertStorageCount(c, s.originalStorageCount)
}

func (s *storageAddSuite) TestAddStorageExceedCount(c *gc.C) {
	_, u, _ := s.setupSingleStorage(c, "block", "loop-pool")
	s.assertStorageCount(c, 1)

	_, err := s.storageBackend.AddStorageForUnit(u.UnitTag(), "data", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, gc.ErrorMatches, `adding "data" storage to storage-block/0: cannot attach, storage is singular`)
	s.assertStorageCount(c, 1)
	s.assertVolumeCount(c, 0)
	s.assertFileSystemCount(c, 0)
}

func (s *storageAddSuite) createAndAssignUnitWithSingleStorage(c *gc.C) names.UnitTag {
	_, u, _ := s.setupSingleStorage(c, "block", "loop-pool")
	s.assertStorageCount(c, 1)

	// Assign unit to machine to get volumes and filesystems
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	volumes, err := s.storageBackend.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	s.originalVolumeCount = len(volumes)

	filesystems, err := s.storageBackend.MachineFilesystemAttachments(s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.originalFilesystemCount = len(filesystems)

	return u.UnitTag()
}

func (s *storageAddSuite) TestAddStorageMinCount(c *gc.C) {
	unit := s.createAndAssignUnitWithSingleStorage(c)
	_, err := s.storageBackend.AddStorageForUnit(unit, "allecto", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, 2)
	s.assertVolumeCount(c, 2)
	s.assertFileSystemCount(c, 0)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}

func (s *storageAddSuite) TestAddStorageZeroCount(c *gc.C) {
	unit := s.createAndAssignUnitWithSingleStorage(c)
	_, err := s.storageBackend.AddStorageForUnit(unit, "allecto", state.StorageConstraints{Pool: "loop-pool", Size: 1024})
	c.Assert(errors.Cause(err), gc.ErrorMatches, "adding storage where instance count is 0 not valid")
	s.assertStorageCount(c, 1)
	s.assertVolumeCount(c, 1)
	s.assertFileSystemCount(c, 0)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}

func (s *storageAddSuite) TestAddStorageTriggerDefaultPopulated(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	s.assignUnit(c, u)

	_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Count: 1})
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, s.originalStorageCount+1)
	s.assertVolumeCount(c, s.originalVolumeCount+1)
	s.assertFileSystemCount(c, s.originalFilesystemCount)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}

func (s *storageAddSuite) TestAddStorageDiffPool(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	s.assignUnit(c, u)

	_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Pool: "loop-pool", Count: 1})
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, s.originalStorageCount+1)
	s.assertVolumeCount(c, s.originalVolumeCount+1)
	s.assertFileSystemCount(c, s.originalFilesystemCount)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}

func (s *storageAddSuite) TestAddStorageDiffSize(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	s.assignUnit(c, u)

	_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Size: 2048, Count: 1})
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, s.originalStorageCount+1)
	s.assertVolumeCount(c, s.originalVolumeCount+1)
	s.assertFileSystemCount(c, s.originalFilesystemCount)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}

func (s *storageAddSuite) TestAddStorageLessMinSize(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	s.assignUnit(c, u)

	_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi2up", state.StorageConstraints{Size: 2, Count: 1})
	c.Assert(err, gc.ErrorMatches, `.*charm "storage-block2" store "multi2up": minimum storage size is 2.0GB, 2.0MB specified.*`)
	s.assertStorageCount(c, s.originalStorageCount)
	s.assertVolumeCount(c, s.originalVolumeCount)
	s.assertFileSystemCount(c, s.originalFilesystemCount)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}

func (s *storageAddSuite) TestAddStorageWrongName(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	s.assignUnit(c, u)

	_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "furball", state.StorageConstraints{Size: 2})
	c.Assert(err, gc.ErrorMatches, `.*charm storage "furball" not found.*`)
	s.assertStorageCount(c, s.originalStorageCount)
	s.assertVolumeCount(c, s.originalVolumeCount)
	s.assertFileSystemCount(c, s.originalFilesystemCount)
}

func (s *storageAddSuite) TestAddStorageConcurrently(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	s.assignUnit(c, u)

	addStorage := func() {
		_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Count: 1})
		c.Assert(err, jc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, addStorage).Check()
	addStorage()
	s.assertStorageCount(c, s.originalStorageCount+2)
	s.assertVolumeCount(c, s.originalVolumeCount+2)
	s.assertFileSystemCount(c, s.originalFilesystemCount)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}

func (s *storageAddSuite) TestAddStorageConcurrentlyExceedCount(c *gc.C) {
	u := s.setupMultipleStoragesForAdd(c)
	s.assignUnit(c, u)

	count := 6
	addStorage := func() {
		_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Count: uint64(count)})
		c.Assert(err, jc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, addStorage).Check()
	_, err := s.storageBackend.AddStorageForUnit(s.unitTag, "multi1to10", state.StorageConstraints{Count: uint64(count)})
	c.Assert(err, gc.ErrorMatches,
		`adding "multi1to10" storage to storage-block2/0: `+
			`attaching 6 storage instances brings the total to 15, exceeding the maximum of 10`)

	// Only "count" number of instances should have been added.
	s.assertStorageCount(c, s.originalStorageCount+count)
	s.assertVolumeCount(c, s.originalVolumeCount+count)
	s.assertFileSystemCount(c, s.originalFilesystemCount)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}

func (s *storageAddSuite) TestAddStorageFilesystem(c *gc.C) {
	_, u, _ := s.setupSingleStorage(c, "filesystem", "loop-pool")

	// Assign unit to machine to get volumes and filesystems
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	machineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	s.machineTag = names.NewMachineTag(machineId)
	s.assertFileSystemCount(c, 1)

	s.assertStorageCount(c, 1)
	s.assertVolumeCount(c, 1)
	s.assertFileSystemCount(c, 1)

	_, err = s.storageBackend.AddStorageForUnit(u.UnitTag(), "data", makeStorageCons("loop-pool", 1024, 1))
	c.Assert(err, jc.ErrorIsNil)
	s.assertStorageCount(c, 2)
	s.assertVolumeCount(c, 2)
	s.assertFileSystemCount(c, 2)
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}

func (s *storageAddSuite) TestAddStorageStatic(c *gc.C) {
	// Create a unit with static storage; ensure that storage-add
	// fails to add more of this kind of storage.
	_, u, _ := s.setupSingleStorage(c, "filesystem", "static")
	s.assertStorageCount(c, 1)

	// Assign unit to machine to get volumes and filesystems
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	machineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	s.machineTag = names.NewMachineTag(machineId)
	s.assertFileSystemCount(c, 1)

	_, err = s.storageBackend.AddStorageForUnit(
		u.UnitTag(), "data",
		makeStorageCons("static", 1024, 1),
	)
	c.Assert(err, gc.ErrorMatches, `adding "data" storage to storage-filesystem/0: `+
		"creating machine storage for storage data/1: "+
		`"static" storage provider does not support dynamic storage`)
	s.assertStorageCount(c, 1)    // no change
	s.assertFileSystemCount(c, 1) // no change
	assertMachineStorageRefs(c, s.storageBackend, s.machineTag)
}
