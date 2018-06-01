// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/status"
)

type FilesystemStateSuite struct {
	StorageStateSuiteBase
}

var _ = gc.Suite(&FilesystemStateSuite{})

func (s *FilesystemStateSuite) TestAddApplicationInvalidPool(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("invalid-pool", 1024, 1),
	}
	_, err := s.State.AddApplication(state.AddApplicationArgs{Name: "storage-filesystem", Charm: ch, Storage: storage})
	c.Assert(err, gc.ErrorMatches, `.* pool "invalid-pool" not found`)
}

func (s *FilesystemStateSuite) TestAddApplicationNoPoolNoDefault(c *gc.C) {
	// no pool specified, no default configured: use rootfs.
	s.testAddApplicationDefaultPool(c, "rootfs", 0)
}

func (s *FilesystemStateSuite) TestAddApplicationNoPoolNoDefaultWithUnits(c *gc.C) {
	// no pool specified, no default configured: use rootfs, add a unit during
	// app deploy.
	s.testAddApplicationDefaultPool(c, "rootfs", 1)
}

func (s *FilesystemStateSuite) TestAddApplicationNoPoolDefaultFilesystem(c *gc.C) {
	// no pool specified, default filesystem configured: use default
	// filesystem.
	err := s.IAASModel.UpdateModelConfig(map[string]interface{}{
		"storage-default-filesystem-source": "machinescoped",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.testAddApplicationDefaultPool(c, "machinescoped", 0)
}

func (s *FilesystemStateSuite) TestAddApplicationNoPoolDefaultBlock(c *gc.C) {
	// no pool specified, default block configured: use default
	// block with managed fs on top.
	err := s.IAASModel.UpdateModelConfig(map[string]interface{}{
		"storage-default-block-source": "modelscoped-block",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.testAddApplicationDefaultPool(c, "modelscoped-block", 0)
}

func (s *FilesystemStateSuite) testAddApplicationDefaultPool(c *gc.C, expectedPool string, numUnits int) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}

	args := state.AddApplicationArgs{
		Name:     "storage-filesystem",
		Charm:    ch,
		Storage:  storage,
		NumUnits: numUnits,
	}
	app, err := s.State.AddApplication(args)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	expected := map[string]state.StorageConstraints{
		"data": state.StorageConstraints{
			Pool:  expectedPool,
			Size:  1024,
			Count: 1,
		},
	}
	c.Assert(cons, jc.DeepEquals, expected)

	app, err = s.State.Application(args.Name)
	c.Assert(err, jc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, numUnits)

	for _, unit := range units {
		scons, err := unit.StorageConstraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(scons, gc.DeepEquals, expected)

		storageAttachments, err := s.IAASModel.UnitStorageAttachments(unit.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(storageAttachments, gc.HasLen, 1)
		storageInstance, err := s.IAASModel.StorageInstance(storageAttachments[0].StorageInstance())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(storageInstance.Kind(), gc.Equals, state.StorageKindFilesystem)
	}
}

func (s *FilesystemStateSuite) TestAddFilesystemWithoutBackingVolume(c *gc.C) {
	s.addUnitWithFilesystem(c, "rootfs", false)
}

func (s *FilesystemStateSuite) TestAddFilesystemWithBackingVolume(c *gc.C) {
	s.addUnitWithFilesystem(c, "modelscoped-block", true)
}

func (s *FilesystemStateSuite) TestSetFilesystemInfoImmutable(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()

	machine := unitMachine(c, s.State, u)
	err = machine.SetProvisioned("inst-id", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	filesystemInfoSet := state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"}
	err = s.IAASModel.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	// The first call to SetFilesystemInfo takes the pool name from
	// the params; the second does not, but it must not change
	// either. Callers are expected to get the existing info and
	// update it, leaving immutable values intact.
	err = s.IAASModel.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
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
	err = s.IAASModel.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem "0/0": filesystem ID not set`)
}

func (s *FilesystemStateSuite) TestVolumeFilesystem(c *gc.C) {
	filesystem, _, _ := s.addUnitWithFilesystem(c, "modelscoped-block", true)
	volumeTag, err := filesystem.Volume()
	c.Assert(err, jc.ErrorIsNil)

	volumeFilesystem := s.volumeFilesystem(c, volumeTag)
	c.Assert(volumeFilesystem.FilesystemTag(), gc.Equals, filesystem.FilesystemTag())
}

func (s *FilesystemStateSuite) addUnitWithFilesystem(c *gc.C, pool string, withVolume bool) (
	state.Filesystem,
	state.FilesystemAttachment,
	state.StorageAttachment,
) {
	filesystem, filesystemAttachment, machine, storageAttachment := s.addUnitWithFilesystemUnprovisioned(
		c, pool, withVolume,
	)

	// Machine must be provisioned before either volume or
	// filesystem can be attached.
	err := machine.SetProvisioned("inst-id", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	if withVolume {
		// Volume must be provisioned before the filesystem.
		volume := s.filesystemVolume(c, filesystem.FilesystemTag())
		err := s.IAASModel.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{VolumeId: "vol-123"})
		c.Assert(err, jc.ErrorIsNil)

		// Volume must be attached before the filesystem.
		err = s.IAASModel.SetVolumeAttachmentInfo(
			machine.MachineTag(),
			volume.VolumeTag(),
			state.VolumeAttachmentInfo{DeviceName: "sdc"},
		)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Filesystem must be provisioned before it can be attached.
	err = s.IAASModel.SetFilesystemInfo(
		filesystem.FilesystemTag(),
		state.FilesystemInfo{FilesystemId: "fs-123"},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.IAASModel.SetFilesystemAttachmentInfo(
		machine.MachineTag(),
		filesystem.FilesystemTag(),
		state.FilesystemAttachmentInfo{MountPoint: "/srv"},
	)
	c.Assert(err, jc.ErrorIsNil)

	return filesystem, filesystemAttachment, storageAttachment
}

func (s *FilesystemStateSuite) addUnitWithFilesystemUnprovisioned(c *gc.C, pool string, withVolume bool) (
	state.Filesystem,
	state.FilesystemAttachment,
	*state.Machine,
	state.StorageAttachment,
) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	app := s.AddTestingApplicationWithStorage(c, "storage-filesystem", ch, storage)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineTag := names.NewMachineTag(assignedMachineId)

	storageAttachments, err := s.IAASModel.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)
	storageInstance, err := s.IAASModel.StorageInstance(storageAttachments[0].StorageInstance())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageInstance.Kind(), gc.Equals, state.StorageKindFilesystem)

	filesystem := s.storageInstanceFilesystem(c, storageInstance.StorageTag())
	filesystemStorageTag, err := filesystem.Storage()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemStorageTag, gc.Equals, storageInstance.StorageTag())
	_, err = filesystem.Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok := filesystem.Params()
	c.Assert(ok, jc.IsTrue)

	volume, err := s.IAASModel.StorageInstanceVolume(storageInstance.StorageTag())
	if withVolume {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volume.VolumeTag(), gc.Equals, names.NewVolumeTag("0"))
		volumeStorageTag, err := volume.StorageInstance()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volumeStorageTag, gc.Equals, storageInstance.StorageTag())
		filesystemVolume, err := filesystem.Volume()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(filesystemVolume, gc.Equals, volume.VolumeTag())
		_, err = s.IAASModel.VolumeAttachment(assignedMachineTag, filesystemVolume)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
		_, err = filesystem.Volume()
		c.Assert(errors.Cause(err), gc.Equals, state.ErrNoBackingVolume)
	}

	machine, err := s.State.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	filesystemAttachments, err := s.IAASModel.MachineFilesystemAttachments(assignedMachineTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemAttachments, gc.HasLen, 1)
	c.Assert(filesystemAttachments[0].Filesystem(), gc.Equals, filesystem.FilesystemTag())
	c.Assert(filesystemAttachments[0].Machine(), gc.Equals, machine.MachineTag())
	_, err = filesystemAttachments[0].Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok = filesystemAttachments[0].Params()
	c.Assert(ok, jc.IsTrue)

	assertMachineStorageRefs(c, s.IAASModel, machine.MachineTag())

	att, err := s.IAASModel.FilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	return filesystem, att, machine, storageAttachments[0]
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

	w := s.IAASModel.WatchFilesystemAttachment(machineTag, filesystemTag)
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	machine, err := s.State.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	// filesystem attachment will NOT react to filesystem changes
	err = s.IAASModel.SetFilesystemInfo(filesystemTag, state.FilesystemInfo{
		FilesystemId: "fs-123",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.IAASModel.SetFilesystemAttachmentInfo(
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
	err = s.IAASModel.SetFilesystemInfo(filesystemTag, filesystemInfo)
	c.Assert(err, jc.ErrorIsNil)
	filesystemInfo.Pool = "rootfs" // taken from params
	s.assertFilesystemInfo(c, filesystemTag, filesystemInfo)
	s.assertFilesystemAttachmentUnprovisioned(c, machineTag, filesystemTag)

	filesystemAttachmentInfo := state.FilesystemAttachmentInfo{MountPoint: "/srv"}
	err = s.IAASModel.SetFilesystemAttachmentInfo(machineTag, filesystemTag, filesystemAttachmentInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertFilesystemAttachmentInfo(c, machineTag, filesystemTag, filesystemAttachmentInfo)
}

func (s *FilesystemStateSuite) TestVolumeBackedFilesystemScope(c *gc.C) {
	_, unit, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped-block")
	err := s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	filesystem := s.storageInstanceFilesystem(c, storageTag)
	c.Assert(filesystem.Tag(), gc.Equals, names.NewFilesystemTag("0"))
	volumeTag, err := filesystem.Volume()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeTag, gc.Equals, names.NewVolumeTag("0"))
}

func (s *FilesystemStateSuite) TestWatchModelFilesystems(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "filesystem")
	addUnit := func() *state.Unit {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
		return u
	}
	u := addUnit()

	w := s.IAASModel.WatchModelFilesystems()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0", "1") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChangeInSingleEvent("4", "5")
	wc.AssertNoChange()

	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	filesystemTag := names.NewFilesystemTag("0")
	removeFilesystemStorageInstance(c, s.IAASModel, filesystemTag)

	err = s.IAASModel.DestroyFilesystem(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0")
	wc.AssertNoChange()

	machineTag := names.NewMachineTag("0")
	err = s.IAASModel.DetachFilesystem(machineTag, filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.IAASModel.RemoveFilesystemAttachment(machineTag, filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0") // last attachment removed
	wc.AssertNoChange()
}

func (s *FilesystemStateSuite) TestWatchEnvironFilesystemAttachments(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "filesystem")
	addUnit := func() *state.Unit {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
		return u
	}
	u := addUnit()

	w := s.IAASModel.WatchModelFilesystemAttachments()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0:0", "0:1") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChangeInSingleEvent("1:4", "1:5")
	wc.AssertNoChange()

	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	filesystemTag := names.NewFilesystemTag("0")
	removeFilesystemStorageInstance(c, s.IAASModel, filesystemTag)

	err = s.IAASModel.DestroyFilesystem(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	machineTag := names.NewMachineTag("0")
	err = s.IAASModel.DetachFilesystem(machineTag, filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0:0")
	wc.AssertNoChange()

	err = s.IAASModel.RemoveFilesystemAttachment(machineTag, filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0:0")
	wc.AssertNoChange()
}

func (s *FilesystemStateSuite) TestWatchMachineFilesystems(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "filesystem")
	addUnit := func() *state.Unit {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
		return u
	}
	u := addUnit()

	w := s.IAASModel.WatchMachineFilesystems(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0/2", "0/3") // initial
	wc.AssertNoChange()

	addUnit()
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	filesystemTag := names.NewFilesystemTag("0/2")
	removeFilesystemStorageInstance(c, s.IAASModel, filesystemTag)

	err = s.IAASModel.DestroyFilesystem(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0/2")
	wc.AssertNoChange()

	attachments, err := s.IAASModel.FilesystemAttachments(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		err := s.IAASModel.DetachFilesystem(a.Machine(), filesystemTag)
		c.Assert(err, jc.ErrorIsNil)
		err = s.IAASModel.RemoveFilesystemAttachment(a.Machine(), filesystemTag)
		c.Assert(err, jc.ErrorIsNil)
	}
	wc.AssertChangeInSingleEvent("0/2") // Dying -> Dead
	wc.AssertNoChange()

	err = s.IAASModel.RemoveFilesystem(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	// no more changes after seeing Dead
	wc.AssertNoChange()
}

func (s *FilesystemStateSuite) TestWatchMachineFilesystemAttachments(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "filesystem", "machinescoped", "modelscoped")
	addUnit := func(to *state.Machine) (u *state.Unit, m *state.Machine) {
		var err error
		u, err = app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		if to != nil {
			err = u.AssignToMachine(to)
			c.Assert(err, jc.ErrorIsNil)
			return u, to
		}
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
		m = unitMachine(c, s.State, u)
		return u, m
	}
	_, m0 := addUnit(nil)

	w := s.IAASModel.WatchMachineFilesystemAttachments(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0:0/0", "0:0/1") // initial
	wc.AssertNoChange()

	addUnit(nil)
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	err := s.IAASModel.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("2"))
	c.Assert(err, jc.ErrorIsNil)
	// no change, since we're only interested in attachments of
	// machine-scoped volumes.
	wc.AssertNoChange()

	removeFilesystemStorageInstance(c, s.IAASModel, names.NewFilesystemTag("0/0"))
	err = s.IAASModel.DestroyFilesystem(names.NewFilesystemTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0:0/0") // dying
	wc.AssertNoChange()

	err = s.IAASModel.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0:0/0") // removed
	wc.AssertNoChange()

	addUnit(m0)
	wc.AssertChangeInSingleEvent("0:0/8", "0:0/9")
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
	assertValid("0/lxd/0:1", names.NewMachineTag("0/lxd/0"), names.NewFilesystemTag("1"))
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

func (s *FilesystemStateSuite) TestRemoveStorageInstanceDestroysAndUnassignsFilesystem(c *gc.C) {
	filesystem, filesystemAttachment, storageAttachment := s.addUnitWithFilesystem(c, "modelscoped-block", true)
	volume := s.filesystemVolume(c, filesystemAttachment.Filesystem())
	storageTag := storageAttachment.StorageInstance()
	unitTag := storageAttachment.Unit()

	err := s.IAASModel.SetFilesystemAttachmentInfo(
		filesystemAttachment.Machine(),
		filesystem.FilesystemTag(),
		state.FilesystemAttachmentInfo{},
	)
	c.Assert(err, jc.ErrorIsNil)

	u, err := s.State.Unit(unitTag.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.IAASModel.DestroyStorageInstance(storageTag, true)
	c.Assert(err, jc.ErrorIsNil)
	err = s.IAASModel.DetachStorage(storageTag, unitTag)
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance and attachment are dying, but not yet
	// removed from state. The filesystem should still be assigned.
	s.storageInstanceFilesystem(c, storageTag)
	s.storageInstanceVolume(c, storageTag)

	err = s.IAASModel.RemoveStorageAttachment(storageTag, unitTag)
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance is now gone; the filesystem should no longer
	// be assigned to the storage.
	_, err = s.IAASModel.StorageInstanceFilesystem(storageTag)
	c.Assert(err, gc.ErrorMatches, `filesystem for storage instance "data/0" not found`)
	_, err = s.IAASModel.StorageInstanceVolume(storageTag)
	c.Assert(err, gc.ErrorMatches, `volume for storage instance "data/0" not found`)

	// The filesystem and volume should still exist. The filesystem
	// should be dying; the volume will be destroyed only once the
	// filesystem is removed.
	f := s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(f.Life(), gc.Equals, state.Dying)
	v := s.volume(c, volume.VolumeTag())
	c.Assert(v.Life(), gc.Equals, state.Alive)
}

func (s *FilesystemStateSuite) TestReleaseStorageInstanceFilesystemReleasing(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystem.Releasing(), jc.IsFalse)
	err = s.IAASModel.SetFilesystemInfo(filesystem.FilesystemTag(), state.FilesystemInfo{FilesystemId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)

	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.IAASModel.ReleaseStorageInstance(storageTag, true)
	c.Assert(err, jc.ErrorIsNil)
	err = s.IAASModel.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	// The filesystem should should be dying, and releasing.
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Dying)
	c.Assert(filesystem.Releasing(), jc.IsTrue)
}

func (s *FilesystemStateSuite) TestReleaseStorageInstanceFilesystemUnreleasable(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped-unreleasable")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystem.Releasing(), jc.IsFalse)
	err = s.IAASModel.SetFilesystemInfo(filesystem.FilesystemTag(), state.FilesystemInfo{FilesystemId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)

	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.IAASModel.ReleaseStorageInstance(storageTag, true)
	c.Assert(err, gc.ErrorMatches,
		`cannot release storage "data/0": storage provider "modelscoped-unreleasable" does not support releasing storage`)
	err = s.IAASModel.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	// The filesystem should should be dying, and releasing.
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Alive)
	c.Assert(filesystem.Releasing(), jc.IsFalse)
}

func (s *FilesystemStateSuite) TestSetFilesystemAttachmentInfoFilesystemNotProvisioned(c *gc.C) {
	_, filesystemAttachment, _, _ := s.addUnitWithFilesystemUnprovisioned(c, "rootfs", false)
	err := s.IAASModel.SetFilesystemAttachmentInfo(
		filesystemAttachment.Machine(),
		filesystemAttachment.Filesystem(),
		state.FilesystemAttachmentInfo{},
	)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem attachment 0/0:0: filesystem "0/0" not provisioned`)
}

func (s *FilesystemStateSuite) TestSetFilesystemAttachmentInfoMachineNotProvisioned(c *gc.C) {
	_, filesystemAttachment, _, _ := s.addUnitWithFilesystemUnprovisioned(c, "rootfs", false)
	err := s.IAASModel.SetFilesystemInfo(
		filesystemAttachment.Filesystem(),
		state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.IAASModel.SetFilesystemAttachmentInfo(
		filesystemAttachment.Machine(),
		filesystemAttachment.Filesystem(),
		state.FilesystemAttachmentInfo{},
	)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem attachment 0/0:0: machine 0 not provisioned`)
}

func (s *FilesystemStateSuite) TestSetFilesystemInfoVolumeAttachmentNotProvisioned(c *gc.C) {
	filesystem, _, _, _ := s.addUnitWithFilesystemUnprovisioned(c, "modelscoped-block", true)
	err := s.IAASModel.SetFilesystemInfo(
		filesystem.FilesystemTag(),
		state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"},
	)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem "0": backing volume "0" is not attached`)
}

func (s *FilesystemStateSuite) TestDestroyFilesystem(c *gc.C) {
	filesystem, _ := s.setupFilesystemAttachment(c, "rootfs")
	assertDestroy := func() {
		s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	}
	defer state.SetBeforeHooks(c, s.State, assertDestroy).Check()
	assertDestroy()
}

func (s *FilesystemStateSuite) TestDestroyFilesystemNotFound(c *gc.C) {
	err := s.IAASModel.DestroyFilesystem(names.NewFilesystemTag("0"))
	c.Assert(err, gc.ErrorMatches, `destroying filesystem 0: filesystem "0" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *FilesystemStateSuite) TestDestroyFilesystemStorageAssigned(c *gc.C) {
	// Create a filesystem-type storage instance, and show that we
	// cannot destroy the filesystem while there is storage assigned.
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	_, err = u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	err = s.IAASModel.DestroyFilesystem(filesystem.FilesystemTag())
	c.Assert(err, gc.ErrorMatches, "destroying filesystem 0/0: filesystem is assigned to storage data/0")

	// We must destroy the unit before we can remove the storage.
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, s.IAASModel, storageTag)
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
}

func (s *FilesystemStateSuite) TestDestroyFilesystemNoAttachments(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")

	err := s.IAASModel.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.IAASModel.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		assertMachineStorageRefs(c, s.IAASModel, machine.MachineTag())
	}).Check()

	// There are no more attachments, so the filesystem should
	// be progressed directly to Dead.
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dead)
}

func (s *FilesystemStateSuite) TestRemoveFilesystem(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	err := s.IAASModel.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.IAASModel.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	assertRemove := func() {
		err = s.IAASModel.RemoveFilesystem(filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		_, err = s.IAASModel.Filesystem(filesystem.FilesystemTag())
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
	defer state.SetBeforeHooks(c, s.State, assertRemove).Check()
	assertRemove()
}

func (s *FilesystemStateSuite) TestRemoveFilesystemVolumeBacked(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped-block")
	volume := s.filesystemVolume(c, filesystem.FilesystemTag())
	assertVolumeLife := func(life state.Life) {
		volume := s.volume(c, volume.VolumeTag())
		c.Assert(volume.Life(), gc.Equals, life)
	}
	assertVolumeAttachmentLife := func(life state.Life) {
		attachment := s.volumeAttachment(c, machine.MachineTag(), volume.VolumeTag())
		c.Assert(attachment.Life(), gc.Equals, life)
	}

	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	// Destroying the filesystem does not trigger destruction
	// of the volume. It cannot be destroyed until all remnants
	// of the filesystem are gone.
	assertVolumeLife(state.Alive)

	err := s.IAASModel.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	// Likewise for the volume attachment.
	assertVolumeAttachmentLife(state.Alive)

	err = s.IAASModel.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	// Removing the filesystem attachment causes the backing-volume
	// to be detached.
	assertVolumeAttachmentLife(state.Dying)

	// Removing the last attachment should cause the filesystem
	// to be removed, since it is volume-backed and dying.
	_, err = s.IAASModel.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	// Removing the filesystem causes the backing-volume to be
	// destroyed.
	assertVolumeLife(state.Dying)

	assertMachineStorageRefs(c, s.IAASModel, machine.MachineTag())
}

func (s *FilesystemStateSuite) TestFilesystemVolumeBackedDestroyDetachVolumeFail(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped-block")
	volume := s.filesystemVolume(c, filesystem.FilesystemTag())

	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	err := s.IAASModel.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	// Can't destroy (detach) volume until the filesystem (attachment) is removed.
	err = s.IAASModel.DetachVolume(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, gc.ErrorMatches, "detaching volume 0 from machine 0: volume contains attached filesystem")
	c.Assert(err, jc.Satisfies, state.IsContainsFilesystem)
	err = s.IAASModel.DestroyVolume(volume.VolumeTag())
	c.Assert(err, gc.ErrorMatches, "destroying volume 0: volume contains filesystem")
	c.Assert(err, jc.Satisfies, state.IsContainsFilesystem)
	assertMachineStorageRefs(c, s.IAASModel, machine.MachineTag())

	err = s.IAASModel.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.IAASModel.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.IAASModel.DetachVolume(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.IAASModel.DestroyVolume(volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestRemoveFilesystemNotFound(c *gc.C) {
	err := s.IAASModel.RemoveFilesystem(names.NewFilesystemTag("42"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestRemoveFilesystemNotDead(c *gc.C) {
	filesystem, _ := s.setupFilesystemAttachment(c, "rootfs")
	err := s.IAASModel.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, gc.ErrorMatches, "removing filesystem 0/0: filesystem is not dead")
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	err = s.IAASModel.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, gc.ErrorMatches, "removing filesystem 0/0: filesystem is not dead")
}

func (s *FilesystemStateSuite) TestDetachFilesystem(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")
	assertDetach := func() {
		err := s.IAASModel.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		attachment := s.filesystemAttachment(c, machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(attachment.Life(), gc.Equals, state.Dying)
	}
	defer state.SetBeforeHooks(c, s.State, assertDetach).Check()
	assertDetach()
}

func (s *FilesystemStateSuite) TestRemoveLastFilesystemAttachment(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")

	err := s.IAASModel.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.IAASModel.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	// The filesystem has no attachments, so it should go straight to Dead.
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dead)
	assertMachineStorageRefs(c, s.IAASModel, machine.MachineTag())
}

func (s *FilesystemStateSuite) TestRemoveLastFilesystemAttachmentConcurrently(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")

	err := s.IAASModel.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	}).Check()

	err = s.IAASModel.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	// Last attachment was removed, and the filesystem was (concurrently)
	// destroyed, so the filesystem should be Dead.
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Dead)
	assertMachineStorageRefs(c, s.IAASModel, machine.MachineTag())
}

func (s *FilesystemStateSuite) TestRemoveFilesystemAttachmentNotFound(c *gc.C) {
	err := s.IAASModel.RemoveFilesystemAttachment(names.NewMachineTag("42"), names.NewFilesystemTag("42"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `removing attachment of filesystem 42 from machine 42: filesystem "42" on machine "42" not found`)
}

func (s *FilesystemStateSuite) TestRemoveFilesystemAttachmentConcurrently(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")
	err := s.IAASModel.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	remove := func() {
		err := s.IAASModel.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		assertMachineStorageRefs(c, s.IAASModel, machine.MachineTag())
	}
	defer state.SetBeforeHooks(c, s.State, remove).Check()
	remove()
}

func (s *FilesystemStateSuite) TestRemoveFilesystemAttachmentAlive(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	err := s.IAASModel.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, gc.ErrorMatches, "removing attachment of filesystem 0/0 from machine 0: filesystem attachment is not dying")
}

func (s *FilesystemStateSuite) TestRemoveMachineRemovesFilesystems(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")

	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	c.Assert(machine.EnsureDead(), jc.ErrorIsNil)
	c.Assert(machine.Remove(), jc.ErrorIsNil)

	// Machine is gone: filesystem should be gone too.
	_, err := s.IAASModel.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	attachments, err := s.IAASModel.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 0)
}

func (s *FilesystemStateSuite) TestDestroyMachineRemovesNonDetachableFilesystems(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "loop")

	// Destroy the machine and run cleanups, which should cause the
	// non-detachable filesystems to be destroyed, detached, and
	// finally removed.
	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	assertCleanupRuns(c, s.State)

	_, err := s.IAASModel.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *FilesystemStateSuite) TestDestroyMachineDetachesDetachableFilesystems(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped-block")

	// Destroy the machine and run cleanups, which should cause the
	// detachable filesystems to be detached, but not destroyed.
	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	assertCleanupRuns(c, s.State)
	s.testDestroyMachineDetachesDetachableFilesystems(
		c, machine.MachineTag(), filesystem.FilesystemTag(),
	)
}

func (s *FilesystemStateSuite) TestDestroyUnitHostMachineDetachesDetachableFilesystems(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped-block")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	machineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(machineId)

	// Destroying the unit should destroy its host machine, which
	// triggers the detachment of storage.
	s.obliterateUnit(c, u.UnitTag())
	assertCleanupRuns(c, s.State)

	s.testDestroyMachineDetachesDetachableFilesystems(
		c, machineTag, filesystem.FilesystemTag(),
	)
}

func (s *FilesystemStateSuite) testDestroyMachineDetachesDetachableFilesystems(
	c *gc.C,
	machineTag names.MachineTag,
	filesystemTag names.FilesystemTag,
) {
	// Filesystem is still alive...
	filesystem, err := s.IAASModel.Filesystem(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystem.Life(), gc.Equals, state.Alive)

	// ... but it has been detached.
	_, err = s.IAASModel.FilesystemAttachment(machineTag, filesystemTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	filesystemStatus, err := filesystem.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemStatus.Status, gc.Equals, status.Detached)
	c.Assert(filesystemStatus.Message, gc.Equals, "")
}

func (s *FilesystemStateSuite) TestDestroyManualMachineDoesntRemoveNonDetachableFilesystems(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "loop")

	// Make this a manual machine, so the cleanup.
	err := machine.SetProvisioned("inst-id", "manual:machine", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy the machine and run cleanups, which should cause the
	// non-detachable filesystems and attachments to be set to Dying,
	// but not completely removed.
	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	assertCleanupRuns(c, s.State)

	filesystem, err = s.IAASModel.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystem.Life(), gc.Equals, state.Dying)
	attachment, err := s.IAASModel.FilesystemAttachment(
		machine.MachineTag(),
		filesystem.FilesystemTag(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachment.Life(), gc.Equals, state.Dying)
}

func (s *FilesystemStateSuite) TestDestroyManualMachineDoesntDetachDetachableFilesystems(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped-block")

	// Make this a manual machine, so the cleanup.
	err := machine.SetProvisioned("inst-id", "manual:machine", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy the machine and run cleanups, which should cause the
	// detachable filesystem attachments to be set to Dying, but not
	// completely removed. The filesystem itself should be left Alive.
	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	assertCleanupRuns(c, s.State)

	filesystem, err = s.IAASModel.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystem.Life(), gc.Equals, state.Alive)
	attachment, err := s.IAASModel.FilesystemAttachment(
		machine.MachineTag(),
		filesystem.FilesystemTag(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachment.Life(), gc.Equals, state.Dying)
}

func (s *FilesystemStateSuite) TestFilesystemMachineScoped(c *gc.C) {
	// Machine-scoped filesystems created unassigned to a storage
	// instance are bound to the machine.
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")

	err := s.IAASModel.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, gc.ErrorMatches, "detaching filesystem 0/0 from machine 0: filesystem is not detachable")
	err = machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.IAASModel.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.IAASModel.FilesystemAttachment(
		machine.MachineTag(),
		filesystem.FilesystemTag(),
	)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *FilesystemStateSuite) TestFilesystemRemoveStorageDestroysFilesystem(c *gc.C) {
	// Filesystems created assigned to a storage instance are bound
	// to the machine/model, and not the storage. i.e. storage is
	// persistent by default.
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	// The filesystem should transition to Dying when the storage is removed.
	// We must destroy the unit before we can remove the storage.
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, s.IAASModel, storageTag)
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Dying)
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

	app := s.AddTestingApplicationWithStorage(c, "storage-filesystem", ch, storage)
	unit, err := app.AddUnit(state.AddUnitParams{})
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

	deploy := func(rev int, location, applicationname string) error {
		ch := s.createStorageCharmRev(c, "storage-filesystem", charm.Storage{
			Name:     "data",
			Type:     charm.StorageFilesystem,
			CountMin: 1,
			CountMax: 1,
			Location: location,
		}, rev)
		app := s.AddTestingApplicationWithStorage(c, applicationname, ch, storage)
		unit, err := app.AddUnit(state.AddUnitParams{})
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
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")

	ch := s.createStorageCharm(c, "storage-filesystem", charm.Storage{
		Name:     "data",
		Type:     charm.StorageFilesystem,
		CountMin: 1,
		CountMax: 1,
		Location: "/not/in/srv",
	})
	app := s.AddTestingApplication(c, "storage-filesystem", ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.IAASModel.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		err = s.IAASModel.RemoveFilesystemAttachment(
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
	app := s.AddTestingApplication(c, "storage-filesystem", ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
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
	app := s.AddTestingApplication(c, "storage-filesystem", ch)

	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, gc.ErrorMatches,
		`cannot assign unit "storage-filesystem/0" to machine 0: `+
			`validating filesystem mount points: `+
			`mount point "/srv" for filesystem 0/0 contains `+
			`mount point "/srv/within" for "data" storage`)
}

func (s *FilesystemStateSuite) TestAddExistingFilesystem(c *gc.C) {
	fsInfoIn := state.FilesystemInfo{
		Pool:         "modelscoped",
		Size:         123,
		FilesystemId: "foo",
	}
	storageTag, err := s.IAASModel.AddExistingFilesystem(fsInfoIn, nil, "pgdata")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageTag, gc.Equals, names.NewStorageTag("pgdata/0"))

	filesystem, err := s.IAASModel.StorageInstanceFilesystem(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	fsInfoOut, err := filesystem.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fsInfoOut, jc.DeepEquals, fsInfoIn)

	fsStatus, err := filesystem.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fsStatus.Status, gc.Equals, status.Detached)
}

func (s *FilesystemStateSuite) TestAddExistingFilesystemEmptyFilesystemId(c *gc.C) {
	fsInfoIn := state.FilesystemInfo{
		Pool: "modelscoped",
		Size: 123,
	}
	_, err := s.IAASModel.AddExistingFilesystem(fsInfoIn, nil, "pgdata")
	c.Assert(err, gc.ErrorMatches, "cannot add existing filesystem: empty filesystem ID not valid")
}

func (s *FilesystemStateSuite) TestAddExistingFilesystemVolumeBacked(c *gc.C) {
	fsInfoIn := state.FilesystemInfo{
		Pool: "modelscoped-block",
		Size: 123,
	}
	volInfoIn := state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "foo",
	}
	storageTag, err := s.IAASModel.AddExistingFilesystem(fsInfoIn, &volInfoIn, "pgdata")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageTag, gc.Equals, names.NewStorageTag("pgdata/0"))

	filesystem, err := s.IAASModel.StorageInstanceFilesystem(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	fsInfoOut, err := filesystem.Info()
	c.Assert(err, jc.ErrorIsNil)
	fsInfoIn.FilesystemId = "filesystem-0" // set by AddExistingFilesystem
	c.Assert(fsInfoOut, jc.DeepEquals, fsInfoIn)

	fsStatus, err := filesystem.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fsStatus.Status, gc.Equals, status.Detached)

	volume, err := s.IAASModel.StorageInstanceVolume(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	volInfoOut, err := volume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volInfoOut, jc.DeepEquals, volInfoIn)

	volStatus, err := volume.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volStatus.Status, gc.Equals, status.Detached)
}

func (s *FilesystemStateSuite) TestAddExistingFilesystemVolumeBackedVolumeInfoMissing(c *gc.C) {
	fsInfo := state.FilesystemInfo{
		Pool:         "modelscoped-block",
		Size:         123,
		FilesystemId: "foo",
	}
	_, err := s.IAASModel.AddExistingFilesystem(fsInfo, nil, "pgdata")
	c.Assert(err, gc.ErrorMatches, "cannot add existing filesystem: backing volume info missing")
}

func (s *FilesystemStateSuite) TestAddExistingFilesystemVolumeBackedFilesystemIdSupplied(c *gc.C) {
	fsInfo := state.FilesystemInfo{
		Pool:         "modelscoped-block",
		Size:         123,
		FilesystemId: "foo",
	}
	volInfo := state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "foo",
	}
	_, err := s.IAASModel.AddExistingFilesystem(fsInfo, &volInfo, "pgdata")
	c.Assert(err, gc.ErrorMatches, "cannot add existing filesystem: non-empty filesystem ID with backing volume not valid")
}

func (s *FilesystemStateSuite) TestAddExistingFilesystemVolumeBackedEmptyVolumeId(c *gc.C) {
	fsInfo := state.FilesystemInfo{
		Pool: "modelscoped-block",
		Size: 123,
	}
	volInfo := state.VolumeInfo{
		Pool: "modelscoped-block",
		Size: 123,
	}
	_, err := s.IAASModel.AddExistingFilesystem(fsInfo, &volInfo, "pgdata")
	c.Assert(err, gc.ErrorMatches, "cannot add existing filesystem: empty backing volume ID not valid")
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
	attachments, err := s.IAASModel.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 1)
	c.Assert(err, jc.ErrorIsNil)
	assertMachineStorageRefs(c, s.IAASModel, machine.MachineTag())
	return s.filesystem(c, attachments[0].Filesystem()), machine
}

func removeFilesystemStorageInstance(c *gc.C, im *state.IAASModel, filesystemTag names.FilesystemTag) {
	filesystem, err := im.Filesystem(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	storageTag, err := filesystem.Storage()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, im, storageTag)
}

func (s *FilesystemStateSuite) assertDestroyFilesystem(c *gc.C, tag names.FilesystemTag, life state.Life) {
	err := s.IAASModel.DestroyFilesystem(tag)
	c.Assert(err, jc.ErrorIsNil)
	filesystem := s.filesystem(c, tag)
	c.Assert(filesystem.Life(), gc.Equals, life)
}
