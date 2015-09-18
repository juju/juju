// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type FilesystemStateSuite struct {
	StorageStateSuiteBase
}

var _ = gc.Suite(&FilesystemStateSuite{})

func (s *FilesystemStateSuite) TestAddServiceInvalidPool(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("invalid-pool", 1024, 1),
	}
	_, err := s.State.AddService("storage-filesystem", s.Owner.String(), ch, nil, storage)
	c.Assert(err, gc.ErrorMatches, `.* pool "invalid-pool" not found`)
}

func (s *FilesystemStateSuite) TestAddServiceNoPoolNoDefault(c *gc.C) {
	// no pool specified, no default configured: use rootfs.
	s.testAddServiceDefaultPool(c, "rootfs")
}

func (s *FilesystemStateSuite) TestAddServiceNoPoolDefaultBlock(c *gc.C) {
	// no pool specified, default block configured: use default
	// block with managed fs on top.
	err := s.State.UpdateEnvironConfig(map[string]interface{}{
		"storage-default-block-source": "machinescoped",
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.testAddServiceDefaultPool(c, "machinescoped")
}

func (s *FilesystemStateSuite) testAddServiceDefaultPool(c *gc.C, expectedPool string) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}
	svc, err := s.State.AddService("storage-filesystem", s.Owner.String(), ch, nil, storage)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := svc.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": state.StorageConstraints{
			Pool:  expectedPool,
			Size:  1024,
			Count: 1,
		},
	})
}

func (s *FilesystemStateSuite) TestAddFilesystemWithoutBackingVolume(c *gc.C) {
	s.addUnitWithFilesystem(c, "rootfs", false)
}

func (s *FilesystemStateSuite) TestAddFilesystemWithBackingVolume(c *gc.C) {
	s.addUnitWithFilesystem(c, "loop", true)
}

func (s *FilesystemStateSuite) TestSetFilesystemInfoImmutable(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()

	assignedMachineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	filesystemInfoSet := state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"}
	err = s.State.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	// The first call to SetFilesystemInfo takes the pool name from
	// the params; the second does not, but it must not change
	// either. Callers are expected to get the existing info and
	// update it, leaving immutable values intact.
	err = s.State.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem "0/0": cannot change pool from "rootfs" to ""`)

	filesystemInfoSet.Pool = "rootfs"
	s.assertFilesystemInfo(c, filesystemTag, filesystemInfoSet)
}

func (s *FilesystemStateSuite) TestSetFilesystemInfoNoFilesystemId(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "loop-pool")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()
	s.assertFilesystemUnprovisioned(c, filesystemTag)

	filesystemInfoSet := state.FilesystemInfo{Size: 123}
	err = s.State.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem "0/0": filesystem ID not set`)
}

func (s *FilesystemStateSuite) TestVolumeFilesystem(c *gc.C) {
	filesystemAttachment, _ := s.addUnitWithFilesystem(c, "loop", true)
	filesystem := s.filesystem(c, filesystemAttachment.Filesystem())
	_, err := filesystem.Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)

	volumeTag, err := filesystem.Volume()
	c.Assert(err, jc.ErrorIsNil)
	filesystem = s.volumeFilesystem(c, volumeTag)
	c.Assert(filesystem.FilesystemTag(), gc.Equals, filesystemAttachment.Filesystem())
}

func (s *FilesystemStateSuite) addUnitWithFilesystem(c *gc.C, pool string, withVolume bool) (state.FilesystemAttachment, state.StorageAttachment) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-filesystem", ch, storage)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineTag := names.NewMachineTag(assignedMachineId)

	storageAttachments, err := s.State.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)
	storageInstance, err := s.State.StorageInstance(storageAttachments[0].StorageInstance())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageInstance.Kind(), gc.Equals, state.StorageKindFilesystem)

	filesystem := s.storageInstanceFilesystem(c, storageInstance.StorageTag())
	c.Assert(filesystem.FilesystemTag(), gc.Equals, names.NewFilesystemTag("0/0"))
	filesystemStorageTag, err := filesystem.Storage()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemStorageTag, gc.Equals, storageInstance.StorageTag())
	_, err = filesystem.Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok := filesystem.Params()
	c.Assert(ok, jc.IsTrue)

	volume, err := s.State.StorageInstanceVolume(storageInstance.StorageTag())
	if withVolume {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volume.VolumeTag(), gc.Equals, names.NewVolumeTag("0/0"))
		volumeStorageTag, err := volume.StorageInstance()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volumeStorageTag, gc.Equals, storageInstance.StorageTag())
		filesystemVolume, err := filesystem.Volume()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(filesystemVolume, gc.Equals, volume.VolumeTag())
		_, err = s.State.VolumeAttachment(assignedMachineTag, filesystemVolume)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
		_, err = filesystem.Volume()
		c.Assert(errors.Cause(err), gc.Equals, state.ErrNoBackingVolume)
	}

	machine, err := s.State.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	filesystemAttachments, err := s.State.MachineFilesystemAttachments(assignedMachineTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemAttachments, gc.HasLen, 1)
	c.Assert(filesystemAttachments[0].Filesystem(), gc.Equals, filesystem.FilesystemTag())
	c.Assert(filesystemAttachments[0].Machine(), gc.Equals, machine.MachineTag())
	_, err = filesystemAttachments[0].Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok = filesystemAttachments[0].Params()
	c.Assert(ok, jc.IsTrue)

	assertMachineStorageRefs(c, s.State, machine.MachineTag())

	att, err := s.State.FilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	return att, storageAttachments[0]
}

func (s *FilesystemStateSuite) TestWatchFilesystemAttachment(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(assignedMachineId)

	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()

	w := s.State.WatchFilesystemAttachment(machineTag, filesystemTag)
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	machine, err := s.State.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	// filesystem attachment will NOT react to filesystem changes
	err = s.State.SetFilesystemInfo(filesystemTag, state.FilesystemInfo{
		FilesystemId: "fs-123",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.State.SetFilesystemAttachmentInfo(
		machineTag, filesystemTag, state.FilesystemAttachmentInfo{
			MountPoint: "/srv",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *FilesystemStateSuite) TestFilesystemInfo(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(assignedMachineId)

	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()

	s.assertFilesystemUnprovisioned(c, filesystemTag)
	s.assertFilesystemAttachmentUnprovisioned(c, machineTag, filesystemTag)

	machine, err := s.State.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	filesystemInfo := state.FilesystemInfo{FilesystemId: "fs-123", Size: 456}
	err = s.State.SetFilesystemInfo(filesystemTag, filesystemInfo)
	c.Assert(err, jc.ErrorIsNil)
	filesystemInfo.Pool = "rootfs" // taken from params
	s.assertFilesystemInfo(c, filesystemTag, filesystemInfo)
	s.assertFilesystemAttachmentUnprovisioned(c, machineTag, filesystemTag)

	filesystemAttachmentInfo := state.FilesystemAttachmentInfo{MountPoint: "/srv"}
	err = s.State.SetFilesystemAttachmentInfo(machineTag, filesystemTag, filesystemAttachmentInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertFilesystemAttachmentInfo(c, machineTag, filesystemTag, filesystemAttachmentInfo)
}

func (s *FilesystemStateSuite) TestVolumeBackedFilesystemScope(c *gc.C) {
	_, unit, storageTag := s.setupSingleStorage(c, "filesystem", "environscoped-block")
	err := s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	filesystem := s.storageInstanceFilesystem(c, storageTag)
	c.Assert(filesystem.Tag(), gc.Equals, names.NewFilesystemTag("0/0"))
	volumeTag, err := filesystem.Volume()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeTag, gc.Equals, names.NewVolumeTag("0"))
}

func (s *FilesystemStateSuite) TestWatchEnvironFilesystems(c *gc.C) {
	service := s.setupMixedScopeStorageService(c, "filesystem")
	addUnit := func() {
		u, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	addUnit()

	w := s.State.WatchEnvironFilesystems()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChangeInSingleEvent("3")
	wc.AssertNoChange()

	// TODO(axw) respond to Dying/Dead when we have
	// the means to progress Filesystem lifecycle.
}

func (s *FilesystemStateSuite) TestWatchEnvironFilesystemAttachments(c *gc.C) {
	service := s.setupMixedScopeStorageService(c, "filesystem")
	addUnit := func() {
		u, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	addUnit()

	w := s.State.WatchEnvironFilesystemAttachments()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0:0") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChangeInSingleEvent("1:3")
	wc.AssertNoChange()

	// TODO(axw) respond to Dying/Dead when we have
	// the means to progress Volume lifecycle.
}

func (s *FilesystemStateSuite) TestWatchMachineFilesystems(c *gc.C) {
	service := s.setupMixedScopeStorageService(c, "filesystem")
	addUnit := func() {
		u, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	addUnit()

	w := s.State.WatchMachineFilesystems(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0/1", "0/2") // initial
	wc.AssertNoChange()

	addUnit()
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	// TODO(axw) respond to Dying/Dead when we have
	// the means to progress Filesystem lifecycle.
}

func (s *FilesystemStateSuite) TestWatchMachineFilesystemAttachments(c *gc.C) {
	service := s.setupMixedScopeStorageService(c, "filesystem")
	addUnit := func(to *state.Machine) (u *state.Unit, m *state.Machine) {
		var err error
		u, err = service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		if to != nil {
			err = u.AssignToMachine(to)
			c.Assert(err, jc.ErrorIsNil)
			return u, to
		}
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
		mid, err := u.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		m, err = s.State.Machine(mid)
		c.Assert(err, jc.ErrorIsNil)
		return u, m
	}
	_, m0 := addUnit(nil)

	w := s.State.WatchMachineFilesystemAttachments(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0:0/1", "0:0/2") // initial
	wc.AssertNoChange()

	addUnit(nil)
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	err := s.State.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	// no change, since we're only interested in attachments of
	// machine-scoped volumes.
	wc.AssertNoChange()

	err = s.State.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("0/1"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0:0/1") // dying
	wc.AssertNoChange()

	err = s.State.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("0/1"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0:0/1") // removed
	wc.AssertNoChange()

	addUnit(m0)
	wc.AssertChangeInSingleEvent("0:0/7", "0:0/8")
	wc.AssertNoChange()
}

func (s *FilesystemStateSuite) TestParseFilesystemAttachmentId(c *gc.C) {
	assertValid := func(id string, m names.MachineTag, v names.FilesystemTag) {
		machineTag, filesystemTag, err := state.ParseFilesystemAttachmentId(id)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(machineTag, gc.Equals, m)
		c.Assert(filesystemTag, gc.Equals, v)
	}
	assertValid("0:0", names.NewMachineTag("0"), names.NewFilesystemTag("0"))
	assertValid("0:0/1", names.NewMachineTag("0"), names.NewFilesystemTag("0/1"))
	assertValid("0/lxc/0:1", names.NewMachineTag("0/lxc/0"), names.NewFilesystemTag("1"))
}

func (s *FilesystemStateSuite) TestParseFilesystemAttachmentIdError(c *gc.C) {
	assertError := func(id, expect string) {
		_, _, err := state.ParseFilesystemAttachmentId(id)
		c.Assert(err, gc.ErrorMatches, expect)
	}
	assertError("", `invalid filesystem attachment ID ""`)
	assertError("0", `invalid filesystem attachment ID "0"`)
	assertError("0:foo", `invalid filesystem attachment ID "0:foo"`)
	assertError("bar:0", `invalid filesystem attachment ID "bar:0"`)
}

func (s *FilesystemStateSuite) TestRemoveStorageInstanceUnassignsFilesystem(c *gc.C) {
	filesystemAttachment, storageAttachment := s.addUnitWithFilesystem(c, "loop", true)
	filesystem := s.filesystem(c, filesystemAttachment.Filesystem())
	volume := s.filesystemVolume(c, filesystemAttachment.Filesystem())
	storageTag := storageAttachment.StorageInstance()
	unitTag := storageAttachment.Unit()

	err := s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DestroyStorageAttachment(storageTag, unitTag)
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance and attachment are dying, but not yet
	// removed from state. The filesystem should still be assigned.
	s.storageInstanceFilesystem(c, storageTag)
	s.storageInstanceVolume(c, storageTag)

	err = s.State.RemoveStorageAttachment(storageTag, unitTag)
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance is now gone; the filesystem should no longer
	// be assigned to the storage.
	_, err = s.State.StorageInstanceFilesystem(storageTag)
	c.Assert(err, gc.ErrorMatches, `filesystem for storage instance "data/0" not found`)
	_, err = s.State.StorageInstanceVolume(storageTag)
	c.Assert(err, gc.ErrorMatches, `volume for storage instance "data/0" not found`)

	// The filesystem and volume should not have been destroyed, though.
	s.filesystem(c, filesystem.FilesystemTag())
	s.volume(c, volume.VolumeTag())
}

func (s *FilesystemStateSuite) TestSetFilesystemAttachmentInfoFilesystemNotProvisioned(c *gc.C) {
	filesystemAttachment, _ := s.addUnitWithFilesystem(c, "rootfs", false)
	err := s.State.SetFilesystemAttachmentInfo(
		filesystemAttachment.Machine(),
		filesystemAttachment.Filesystem(),
		state.FilesystemAttachmentInfo{},
	)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem attachment 0/0:0: filesystem "0/0" not provisioned`)
}

func (s *FilesystemStateSuite) TestSetFilesystemAttachmentInfoMachineNotProvisioned(c *gc.C) {
	filesystemAttachment, _ := s.addUnitWithFilesystem(c, "rootfs", false)
	err := s.State.SetFilesystemInfo(
		filesystemAttachment.Filesystem(),
		state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetFilesystemAttachmentInfo(
		filesystemAttachment.Machine(),
		filesystemAttachment.Filesystem(),
		state.FilesystemAttachmentInfo{},
	)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem attachment 0/0:0: machine 0 not provisioned`)
}

func (s *FilesystemStateSuite) TestSetFilesystemInfoVolumeAttachmentNotProvisioned(c *gc.C) {
	filesystemAttachment, _ := s.addUnitWithFilesystem(c, "loop", true)
	err := s.State.SetFilesystemInfo(
		filesystemAttachment.Filesystem(),
		state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"},
	)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem "0/0": volume attachment "0/0" on "0" not provisioned`)
}

func (s *FilesystemStateSuite) TestDestroyFilesystem(c *gc.C) {
	filesystem, _ := s.setupFilesystemAttachment(c, "rootfs")
	assertDestroy := func() {
		err := s.State.DestroyFilesystem(filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		filesystem = s.filesystem(c, filesystem.FilesystemTag())
		c.Assert(filesystem.Life(), gc.Equals, state.Dying)
	}
	defer state.SetBeforeHooks(c, s.State, assertDestroy).Check()
	assertDestroy()
}

func (s *FilesystemStateSuite) TestDestroyFilesystemNoAttachments(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")

	err := s.State.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		assertMachineStorageRefs(c, s.State, machine.MachineTag())
	}).Check()

	err = s.State.DestroyFilesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	filesystem = s.filesystem(c, filesystem.FilesystemTag())

	// There are no more attachments, so the filesystem should
	// have been progressed directly to Dead.
	c.Assert(filesystem.Life(), gc.Equals, state.Dead)
}

func (s *FilesystemStateSuite) TestRemoveFilesystem(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	err := s.State.DestroyFilesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	assertRemove := func() {
		err = s.State.RemoveFilesystem(filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		_, err = s.State.Filesystem(filesystem.FilesystemTag())
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
	defer state.SetBeforeHooks(c, s.State, assertRemove).Check()
	assertRemove()
}

func (s *FilesystemStateSuite) TestRemoveFilesystemVolumeBacked(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "loop")
	volume := s.filesystemVolume(c, filesystem.FilesystemTag())
	assertVolumeLife := func(life state.Life) {
		volume := s.volume(c, volume.VolumeTag())
		c.Assert(volume.Life(), gc.Equals, life)
	}
	assertVolumeAttachmentLife := func(life state.Life) {
		attachment := s.volumeAttachment(c, machine.MachineTag(), volume.VolumeTag())
		c.Assert(attachment.Life(), gc.Equals, life)
	}

	err := s.State.DestroyFilesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	// Destroying the filesystem does not trigger destruction
	// of the volume. It cannot be destroyed until all remnants
	// of the filesystem are gone.
	assertVolumeLife(state.Alive)

	err = s.State.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	// Likewise for the volume attachment.
	assertVolumeAttachmentLife(state.Alive)

	err = s.State.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	// Removing the filesystem attachment causes the backing-volume
	// to be detached.
	assertVolumeAttachmentLife(state.Dying)

	err = s.State.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	// Removing the filesystem causes the backing-volume to be
	// destroyed.
	assertVolumeLife(state.Dying)

	assertMachineStorageRefs(c, s.State, machine.MachineTag())
}

func (s *FilesystemStateSuite) TestFilesystemVolumeBackedDestroyDetachVolumeFail(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "loop")
	volume := s.filesystemVolume(c, filesystem.FilesystemTag())

	err := s.State.DestroyFilesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	// Can't destroy (detach) volume until the filesystem (attachment) is removed.
	err = s.State.DetachVolume(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, gc.ErrorMatches, "detaching volume 0/0 from machine 0: volume contains attached filesystem")
	c.Assert(err, jc.Satisfies, state.IsContainsFilesystem)
	err = s.State.DestroyVolume(volume.VolumeTag())
	c.Assert(err, gc.ErrorMatches, "destroying volume 0/0: volume contains filesystem")
	c.Assert(err, jc.Satisfies, state.IsContainsFilesystem)
	assertMachineStorageRefs(c, s.State, machine.MachineTag())

	err = s.State.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.DetachVolume(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DestroyVolume(volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestRemoveFilesystemNotFound(c *gc.C) {
	err := s.State.RemoveFilesystem(names.NewFilesystemTag("42"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestRemoveFilesystemNotDead(c *gc.C) {
	filesystem, _ := s.setupFilesystemAttachment(c, "rootfs")
	err := s.State.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, gc.ErrorMatches, "removing filesystem 0/0: filesystem is not dead")
	err = s.State.DestroyFilesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, gc.ErrorMatches, "removing filesystem 0/0: filesystem is not dead")
}

func (s *FilesystemStateSuite) TestDetachFilesystem(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	assertDetach := func() {
		err := s.State.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		attachment := s.filesystemAttachment(c, machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(attachment.Life(), gc.Equals, state.Dying)
	}
	defer state.SetBeforeHooks(c, s.State, assertDetach).Check()
	assertDetach()
}

func (s *FilesystemStateSuite) TestRemoveLastFilesystemAttachment(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")

	err := s.State.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.DestroyFilesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	// The filesystem had no attachments when it was destroyed,
	// so it should be Dead.
	c.Assert(filesystem.Life(), gc.Equals, state.Dead)
	assertMachineStorageRefs(c, s.State, machine.MachineTag())
}

func (s *FilesystemStateSuite) TestRemoveLastFilesystemAttachmentConcurrently(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")

	err := s.State.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.DestroyFilesystem(filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		filesystem := s.filesystem(c, filesystem.FilesystemTag())
		c.Assert(filesystem.Life(), gc.Equals, state.Dying)
	}).Check()

	err = s.State.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	// Last attachment was removed, and the filesystem was (concurrently)
	// destroyed, so the filesystem should be Dead.
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Dead)
	assertMachineStorageRefs(c, s.State, machine.MachineTag())
}

func (s *FilesystemStateSuite) TestRemoveFilesystemAttachmentNotFound(c *gc.C) {
	err := s.State.RemoveFilesystemAttachment(names.NewMachineTag("42"), names.NewFilesystemTag("42"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `removing attachment of filesystem 42 from machine 42: filesystem "42" on machine "42" not found`)
}

func (s *FilesystemStateSuite) TestRemoveFilesystemAttachmentConcurrently(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	err := s.State.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	remove := func() {
		err := s.State.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		assertMachineStorageRefs(c, s.State, machine.MachineTag())
	}
	defer state.SetBeforeHooks(c, s.State, remove).Check()
	remove()
}

func (s *FilesystemStateSuite) TestRemoveFilesystemAttachmentAlive(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	err := s.State.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, gc.ErrorMatches, "removing attachment of filesystem 0/0 from machine 0: filesystem attachment is not dying")
}

func (s *FilesystemStateSuite) TestRemoveMachineRemovesFilesystems(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")

	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	c.Assert(machine.EnsureDead(), jc.ErrorIsNil)
	c.Assert(machine.Remove(), jc.ErrorIsNil)

	// Machine is gone: filesystem should be gone too.
	_, err := s.State.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	attachments, err := s.State.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 0)
}

func (s *FilesystemStateSuite) TestFilesystemBindingMachine(c *gc.C) {
	// Filesystems created unassigned to a storage instance are
	// bound to the initially attached machine.
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	c.Assert(filesystem.LifeBinding(), gc.Equals, machine.Tag())

	err := s.State.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Dead)

	// TODO(axw) when we can assign storage to an existing filesystem, we
	// should test that a machine-bound filesystem is not destroyed when
	// its assigned storage instance is removed.
}

func (s *FilesystemStateSuite) TestFilesystemBindingStorage(c *gc.C) {
	// Filesystems created assigned to a storage instance are bound
	// to the storage instance.
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	c.Assert(filesystem.LifeBinding(), gc.Equals, storageTag)

	err = s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	attachments, err := s.State.StorageAttachments(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		err = s.State.DestroyStorageAttachment(storageTag, a.Unit())
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.RemoveStorageAttachment(storageTag, a.Unit())
		c.Assert(err, jc.ErrorIsNil)
	}

	// The storage instance should be removed,
	// and the filesystem should be Dying.
	_, err = s.State.StorageInstance(storageTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Dying)
}

func (s *FilesystemStateSuite) TestFilesystemVolumeBinding(c *gc.C) {
	// A volume backing a filesystem is bound to the filesystem.
	filesystem, _ := s.setupFilesystemAttachment(c, "loop")
	volume := s.filesystemVolume(c, filesystem.FilesystemTag())
	c.Assert(volume.LifeBinding(), gc.Equals, filesystem.Tag())

	// TestRemoveFilesystemVolumeBacked tests that removal of
	// filesystem destroys volume.
}

func (s *FilesystemStateSuite) TestEnsureMachineDeadAddFilesystemConcurrently(c *gc.C) {
	_, machine := s.setupFilesystemAttachment(c, "rootfs")
	addFilesystem := func() {
		_, u, _ := s.setupSingleStorage(c, "filesystem", "rootfs")
		err := u.AssignToMachine(machine)
		c.Assert(err, jc.ErrorIsNil)
		s.obliterateUnit(c, u.UnitTag())
	}
	defer state.SetBeforeHooks(c, s.State, addFilesystem).Check()

	// Adding another filesystem to the machine will cause EnsureDead to
	// retry, but it will succeed because both filesystems are inherently
	// machine-bound.
	err := machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestEnsureMachineDeadRemoveFilesystemConcurrently(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	removeFilesystem := func() {
		s.obliterateFilesystem(c, filesystem.FilesystemTag())
	}
	defer state.SetBeforeHooks(c, s.State, removeFilesystem).Check()

	// Removing a filesystem concurrently does not cause a transaction failure.
	err := machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsSingletonNoLocation(c *gc.C) {
	s.testFilesystemAttachmentParams(c, 0, 1, "", state.FilesystemAttachmentParams{
		Location: "/var/lib/juju/storage/data/0",
	})
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsMultipleNoLocation(c *gc.C) {
	s.testFilesystemAttachmentParams(c, 0, -1, "", state.FilesystemAttachmentParams{
		Location: "/var/lib/juju/storage/data/0",
	})
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsSingletonLocation(c *gc.C) {
	s.testFilesystemAttachmentParams(c, 0, 1, "/srv", state.FilesystemAttachmentParams{
		Location: "/srv",
	})
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsMultipleLocation(c *gc.C) {
	s.testFilesystemAttachmentParams(c, 0, -1, "/srv", state.FilesystemAttachmentParams{
		Location: "/srv/data/0",
	})
}

func (s *FilesystemStateSuite) testFilesystemAttachmentParams(
	c *gc.C, countMin, countMax int, location string,
	expect state.FilesystemAttachmentParams,
) {
	ch := s.createStorageCharm(c, "storage-filesystem", charm.Storage{
		Name:     "data",
		Type:     charm.StorageFilesystem,
		CountMin: countMin,
		CountMax: countMax,
		Location: location,
	})
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("rootfs", 1024, 1),
	}

	service := s.AddTestingServiceWithStorage(c, "storage-filesystem", ch, storage)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	storageTag := names.NewStorageTag("data/0")
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemAttachment := s.filesystemAttachment(
		c, names.NewMachineTag(machineId), filesystem.FilesystemTag(),
	)
	params, ok := filesystemAttachment.Params()
	c.Assert(ok, jc.IsTrue)
	c.Assert(params, jc.DeepEquals, expect)
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsLocationConflictConcurrent(c *gc.C) {
	s.testFilesystemAttachmentParamsConcurrent(
		c, "/srv", "/srv",
		`cannot assign unit "storage-filesystem-after/0" to machine 0: `+
			`validating filesystem mount points: `+
			`mount point "/srv" for "data" storage contains mount point "/srv" for "data" storage`)
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsLocationAutoConcurrent(c *gc.C) {
	s.testFilesystemAttachmentParamsConcurrent(c, "", "", "")
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsLocationAutoAndManualConcurrent(c *gc.C) {
	s.testFilesystemAttachmentParamsConcurrent(c, "", "/srv", "")
}

func (s *FilesystemStateSuite) testFilesystemAttachmentParamsConcurrent(c *gc.C, locBefore, locAfter, expectErr string) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("rootfs", 1024, 1),
	}

	deploy := func(rev int, location, serviceName string) error {
		ch := s.createStorageCharmRev(c, "storage-filesystem", charm.Storage{
			Name:     "data",
			Type:     charm.StorageFilesystem,
			CountMin: 1,
			CountMax: 1,
			Location: location,
		}, rev)
		service := s.AddTestingServiceWithStorage(c, serviceName, ch, storage)
		unit, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		return unit.AssignToMachine(machine)
	}

	defer state.SetBeforeHooks(c, s.State, func() {
		err := deploy(1, locBefore, "storage-filesystem-before")
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = deploy(2, locAfter, "storage-filesystem-after")
	if expectErr != "" {
		c.Assert(err, gc.ErrorMatches, expectErr)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsConcurrentRemove(c *gc.C) {
	// this creates a filesystem mounted at "/srv".
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")

	ch := s.createStorageCharm(c, "storage-filesystem", charm.Storage{
		Name:     "data",
		Type:     charm.StorageFilesystem,
		CountMin: 1,
		CountMax: 1,
		Location: "/not/in/srv",
	})
	service := s.AddTestingService(c, "storage-filesystem", ch)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.RemoveFilesystemAttachment(
			machine.MachineTag(), filesystem.FilesystemTag(),
		)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsLocationStorageDir(c *gc.C) {
	ch := s.createStorageCharm(c, "storage-filesystem", charm.Storage{
		Name:     "data",
		Type:     charm.StorageFilesystem,
		CountMin: 1,
		CountMax: 1,
		Location: "/var/lib/juju/storage",
	})
	service := s.AddTestingService(c, "storage-filesystem", ch)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, gc.ErrorMatches, `cannot assign unit \"storage-filesystem/0\" to machine: `+
		`cannot assign unit "storage-filesystem/0" to clean, empty machine: `+
		`getting filesystem mount point for storage data: `+
		`invalid location "/var/lib/juju/storage": `+
		`must not fall within "/var/lib/juju/storage"`)
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentLocationConflict(c *gc.C) {
	// this creates a filesystem mounted at "/srv".
	_, machine := s.setupFilesystemAttachment(c, "rootfs")

	ch := s.createStorageCharm(c, "storage-filesystem", charm.Storage{
		Name:     "data",
		Type:     charm.StorageFilesystem,
		CountMin: 1,
		CountMax: 1,
		Location: "/srv/within",
	})
	svc := s.AddTestingService(c, "storage-filesystem", ch)

	u, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, gc.ErrorMatches,
		`cannot assign unit "storage-filesystem/0" to machine 0: `+
			`validating filesystem mount points: `+
			`mount point "/srv" for filesystem 0/0 contains `+
			`mount point "/srv/within" for "data" storage`)
}

func (s *FilesystemStateSuite) setupFilesystemAttachment(c *gc.C, pool string) (state.Filesystem, *state.Machine) {
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Filesystems: []state.MachineFilesystemParams{{
			Filesystem: state.FilesystemParams{Pool: pool, Size: 1024},
			Attachment: state.FilesystemAttachmentParams{
				Location: "/srv",
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	attachments, err := s.State.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 1)
	c.Assert(err, jc.ErrorIsNil)
	assertMachineStorageRefs(c, s.State, machine.MachineTag())
	return s.filesystem(c, attachments[0].Filesystem()), machine
}
