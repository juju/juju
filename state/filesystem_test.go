// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type FilesystemStateSuite struct {
	StorageStateSuiteBase
}

type FilesystemIAASModelSuite struct {
	FilesystemStateSuite
}

type FilesystemCAASModelSuite struct {
	FilesystemStateSuite
}

var _ = gc.Suite(&FilesystemIAASModelSuite{})
var _ = gc.Suite(&FilesystemCAASModelSuite{})

func (s *FilesystemCAASModelSuite) SetUpTest(c *gc.C) {
	s.series = "kubernetes"
	s.FilesystemStateSuite.SetUpTest(c)
	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)
}

func (s *FilesystemStateSuite) TestAddApplicationInvalidPool(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("invalid-pool", 1024, 1),
	}
	_, err := s.st.AddApplication(state.AddApplicationArgs{
		Name: "storage-filesystem", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: storage,
	})
	c.Assert(err, gc.ErrorMatches, `.* pool "invalid-pool" not found`)
}

func (s *FilesystemStateSuite) TestAddApplicationNoPoolNoDefault(c *gc.C) {
	// no pool specified, no default configured: use default.
	expected := "rootfs"
	if s.series == "kubernetes" {
		expected = "kubernetes"
	}
	s.testAddApplicationDefaultPool(c, expected, 0)
}

func (s *FilesystemStateSuite) TestAddApplicationNoPoolNoDefaultWithUnits(c *gc.C) {
	// no pool specified, no default configured: use default, add a unit during
	// app deploy.
	expected := "rootfs"
	if s.series == "kubernetes" {
		expected = "kubernetes"
	}
	s.testAddApplicationDefaultPool(c, expected, 1)
}

func (s *FilesystemIAASModelSuite) TestAddApplicationNoPoolDefaultFilesystem(c *gc.C) {
	// no pool specified, default filesystem configured: use default
	// filesystem.
	m, err := s.st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = m.UpdateModelConfig(map[string]interface{}{
		"storage-default-filesystem-source": "machinescoped",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.testAddApplicationDefaultPool(c, "machinescoped", 0)
}

func (s *FilesystemIAASModelSuite) TestAddApplicationNoPoolDefaultBlock(c *gc.C) {
	// no pool specified, default block configured: use default
	// block with managed fs on top.
	m, err := s.st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = m.UpdateModelConfig(map[string]interface{}{
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
		Name:  "storage-filesystem",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage:  storage,
		NumUnits: numUnits,
	}
	app, err := s.st.AddApplication(args)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	expected := map[string]state.StorageConstraints{
		"data": {
			Pool:  expectedPool,
			Size:  1024,
			Count: 1,
		},
	}
	if s.series == "kubernetes" {
		expected["cache"] = state.StorageConstraints{Count: 0, Size: 1024, Pool: expectedPool}
	}
	c.Assert(cons, jc.DeepEquals, expected)

	app, err = s.st.Application(args.Name)
	c.Assert(err, jc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, numUnits)

	for _, unit := range units {
		scons, err := unit.StorageConstraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(scons, gc.DeepEquals, expected)

		storageAttachments, err := s.storageBackend.UnitStorageAttachments(unit.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(storageAttachments, gc.HasLen, 1)
		storageInstance, err := s.storageBackend.StorageInstance(storageAttachments[0].StorageInstance())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(storageInstance.Kind(), gc.Equals, state.StorageKindFilesystem)
	}
}

func (s *FilesystemStateSuite) TestAddFilesystemWithoutBackingVolume(c *gc.C) {
	s.addUnitWithFilesystem(c, "rootfs", false)
}

func (s *FilesystemIAASModelSuite) TestAddFilesystemWithBackingVolume(c *gc.C) {
	s.addUnitWithFilesystem(c, "modelscoped-block", true)
}

func (s *FilesystemStateSuite) TestSetFilesystemInfoImmutable(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	hostTag := s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()

	if _, ok := hostTag.(names.MachineTag); ok {
		machine := unitMachine(c, s.st, u)
		err := machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	filesystemInfoSet := state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"}
	err := s.storageBackend.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	// The first call to SetFilesystemInfo takes the pool name from
	// the params; the second does not, but it must not change
	// either. Callers are expected to get the existing info and
	// update it, leaving immutable values intact.
	err = s.storageBackend.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem ".*0/0": cannot change pool from "rootfs" to ""`)

	filesystemInfoSet.Pool = "rootfs"
	s.assertFilesystemInfo(c, filesystemTag, filesystemInfoSet)
}

func (s *FilesystemStateSuite) maybeAssignUnit(c *gc.C, u *state.Unit) names.Tag {
	m, err := s.st.Model()
	c.Assert(err, jc.ErrorIsNil)
	if m.Type() == state.ModelTypeCAAS {
		return u.UnitTag()
	}
	err = s.st.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	machineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	return names.NewMachineTag(machineId)
}

func (s *FilesystemStateSuite) TestSetFilesystemInfoNoFilesystemId(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "tmpfs-pool")
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()
	s.assertFilesystemUnprovisioned(c, filesystemTag)

	filesystemInfoSet := state.FilesystemInfo{Size: 123}
	err := s.storageBackend.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem ".*0/0": filesystem ID not set`)
}

func (s *FilesystemIAASModelSuite) TestVolumeFilesystem(c *gc.C) {
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
	filesystem, filesystemAttachment, storageAttachment := s.addUnitWithFilesystemUnprovisioned(
		c, pool, withVolume,
	)

	if machineTag, ok := filesystemAttachment.Host().(names.MachineTag); ok {
		// Machine must be provisioned before either volume or
		// filesystem can be attached.
		machine, err := s.st.Machine(machineTag.Id())
		c.Assert(err, jc.ErrorIsNil)
		err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	if withVolume {
		// Volume must be provisioned before the filesystem.
		volume := s.filesystemVolume(c, filesystem.FilesystemTag())
		err := s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{VolumeId: "vol-123"})
		c.Assert(err, jc.ErrorIsNil)

		// Volume must be attached before the filesystem.
		err = s.storageBackend.SetVolumeAttachmentInfo(
			filesystemAttachment.Host(),
			volume.VolumeTag(),
			state.VolumeAttachmentInfo{DeviceName: "sdc"},
		)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Filesystem must be provisioned before it can be attached.
	err := s.storageBackend.SetFilesystemInfo(
		filesystem.FilesystemTag(),
		state.FilesystemInfo{FilesystemId: "fs-123"},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.storageBackend.SetFilesystemAttachmentInfo(
		filesystemAttachment.Host(),
		filesystem.FilesystemTag(),
		state.FilesystemAttachmentInfo{MountPoint: "/srv"},
	)
	c.Assert(err, jc.ErrorIsNil)

	return filesystem, filesystemAttachment, storageAttachment
}

func (s *FilesystemStateSuite) addUnitWithFilesystemUnprovisioned(c *gc.C, pool string, withVolume bool) (
	state.Filesystem,
	state.FilesystemAttachment,
	state.StorageAttachment,
) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	app := s.AddTestingApplicationWithStorage(c, "storage-filesystem", ch, storage)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	hostTag := s.maybeAssignUnit(c, unit)

	storageAttachments, err := s.storageBackend.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)
	storageInstance, err := s.storageBackend.StorageInstance(storageAttachments[0].StorageInstance())
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

	volume, err := s.storageBackend.StorageInstanceVolume(storageInstance.StorageTag())
	if withVolume {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volume.VolumeTag(), gc.Equals, names.NewVolumeTag("0"))
		volumeStorageTag, err := volume.StorageInstance()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volumeStorageTag, gc.Equals, storageInstance.StorageTag())
		filesystemVolume, err := filesystem.Volume()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(filesystemVolume, gc.Equals, volume.VolumeTag())
		_, err = s.storageBackend.VolumeAttachment(hostTag, filesystemVolume)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
		_, err = filesystem.Volume()
		c.Assert(errors.Cause(err), gc.Equals, state.ErrNoBackingVolume)
	}

	if s.series != "kubernetes" {
		machineTag := hostTag.(names.MachineTag)
		filesystemAttachments, err := s.storageBackend.MachineFilesystemAttachments(machineTag)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(filesystemAttachments, gc.HasLen, 1)
		c.Assert(filesystemAttachments[0].Filesystem(), gc.Equals, filesystem.FilesystemTag())
		c.Assert(filesystemAttachments[0].Host(), gc.Equals, hostTag)
		_, err = filesystemAttachments[0].Info()
		c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
		_, ok = filesystemAttachments[0].Params()
		c.Assert(ok, jc.IsTrue)

		assertMachineStorageRefs(c, s.storageBackend, machineTag)
	}

	att, err := s.storageBackend.FilesystemAttachment(hostTag, filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	return filesystem, att, storageAttachments[0]
}

func (s *FilesystemIAASModelSuite) TestWatchFilesystemAttachment(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	err := s.st.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(assignedMachineId)

	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchFilesystemAttachment(machineTag, filesystemTag)
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	machine, err := s.st.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	// filesystem attachment will NOT react to filesystem changes
	err = s.storageBackend.SetFilesystemInfo(filesystemTag, state.FilesystemInfo{
		FilesystemId: "fs-123",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.storageBackend.SetFilesystemAttachmentInfo(
		machineTag, filesystemTag, state.FilesystemAttachmentInfo{
			MountPoint: "/srv",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *FilesystemStateSuite) TestFilesystemInfo(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	hostTag := s.maybeAssignUnit(c, u)

	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()

	s.assertFilesystemUnprovisioned(c, filesystemTag)
	s.assertFilesystemAttachmentUnprovisioned(c, hostTag, filesystemTag)

	if _, ok := hostTag.(names.MachineTag); ok {
		machine, err := s.st.Machine(hostTag.Id())
		c.Assert(err, jc.ErrorIsNil)
		err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	filesystemInfo := state.FilesystemInfo{FilesystemId: "fs-123", Size: 456}
	err := s.storageBackend.SetFilesystemInfo(filesystemTag, filesystemInfo)
	c.Assert(err, jc.ErrorIsNil)
	filesystemInfo.Pool = "rootfs" // taken from params
	s.assertFilesystemInfo(c, filesystemTag, filesystemInfo)
	s.assertFilesystemAttachmentUnprovisioned(c, hostTag, filesystemTag)

	filesystemAttachmentInfo := state.FilesystemAttachmentInfo{MountPoint: "/srv"}
	err = s.storageBackend.SetFilesystemAttachmentInfo(hostTag, filesystemTag, filesystemAttachmentInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertFilesystemAttachmentInfo(c, hostTag, filesystemTag, filesystemAttachmentInfo)
}

func (s *FilesystemIAASModelSuite) TestVolumeBackedFilesystemScope(c *gc.C) {
	_, unit, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped-block")
	err := s.st.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	filesystem := s.storageInstanceFilesystem(c, storageTag)
	c.Assert(filesystem.Tag(), gc.Equals, names.NewFilesystemTag("0"))
	volumeTag, err := filesystem.Volume()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeTag, gc.Equals, names.NewVolumeTag("0"))
}

func (s *FilesystemIAASModelSuite) TestWatchModelFilesystems(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "filesystem")
	addUnit := func() *state.Unit {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.st.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
		return u
	}
	u := addUnit()
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchModelFilesystems()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0", "1") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChange("4", "5")
	wc.AssertNoChange()

	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	filesystemTag := names.NewFilesystemTag("0")
	removeFilesystemStorageInstance(c, s.storageBackend, filesystemTag)

	err = s.storageBackend.DestroyFilesystem(filesystemTag, false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	machineTag := names.NewMachineTag("0")
	err = s.storageBackend.DetachFilesystem(machineTag, filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystemAttachment(machineTag, filesystemTag, false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0") // last attachment removed
	wc.AssertNoChange()
}

func (s *FilesystemIAASModelSuite) TestWatchModelFilesystemAttachments(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "filesystem")
	addUnit := func() *state.Unit {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.st.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
		return u
	}
	u := addUnit()
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchModelFilesystemAttachments()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0:0", "0:1") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChange("1:4", "1:5")
	wc.AssertNoChange()

	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	filesystemTag := names.NewFilesystemTag("0")
	removeFilesystemStorageInstance(c, s.storageBackend, filesystemTag)

	err = s.storageBackend.DestroyFilesystem(filesystemTag, false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	machineTag := names.NewMachineTag("0")
	err = s.storageBackend.DetachFilesystem(machineTag, filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0:0")
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystemAttachment(machineTag, filesystemTag, false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0:0")
	wc.AssertNoChange()
}

func (s *FilesystemIAASModelSuite) TestWatchMachineFilesystems(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "filesystem")
	addUnit := func() *state.Unit {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.st.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
		return u
	}
	u := addUnit()
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchMachineFilesystems(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0/2", "0/3") // initial
	wc.AssertNoChange()

	addUnit()
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	filesystemTag := names.NewFilesystemTag("0/2")
	removeFilesystemStorageInstance(c, s.storageBackend, filesystemTag)

	err = s.storageBackend.DestroyFilesystem(filesystemTag, false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0/2")
	wc.AssertNoChange()

	attachments, err := s.storageBackend.FilesystemAttachments(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		err := s.storageBackend.DetachFilesystem(a.Host(), filesystemTag)
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.RemoveFilesystemAttachment(a.Host(), filesystemTag, false)
		c.Assert(err, jc.ErrorIsNil)
	}
	wc.AssertChange("0/2") // Dying -> Dead
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystem(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	// no more changes after seeing Dead
	wc.AssertNoChange()
}

func (s *FilesystemIAASModelSuite) TestWatchMachineFilesystemAttachments(c *gc.C) {
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
		err = s.st.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
		m = unitMachine(c, s.st, u)
		return u, m
	}
	_, m0 := addUnit(nil)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchMachineFilesystemAttachments(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0:0/0", "0:0/1") // initial
	wc.AssertNoChange()

	addUnit(nil)
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	err := s.storageBackend.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("2"))
	c.Assert(err, jc.ErrorIsNil)
	// no change, since we're only interested in attachments of
	// machine-scoped volumes.
	wc.AssertNoChange()

	removeFilesystemStorageInstance(c, s.storageBackend, names.NewFilesystemTag("0/0"))
	err = s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("0/0"), false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0:0/0") // dying
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("0/0"), false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0:0/0") // removed
	wc.AssertNoChange()

	addUnit(m0)
	wc.AssertChange("0:0/8", "0:0/9")
	wc.AssertNoChange()
}

func (s *FilesystemCAASModelSuite) TestWatchUnitFilesystems(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data":  {Count: 1, Size: 1024, Pool: "kubernetes"},
		"cache": {Count: 1, Size: 1024, Pool: "rootfs"},
	}
	app, err := s.st.AddApplication(state.AddApplicationArgs{
		Name: "mariadb", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: storage,
	})
	c.Assert(err, jc.ErrorIsNil)

	addUnit := func(app *state.Application) *state.Unit {
		var err error
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		return u
	}
	u := addUnit(app)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchUnitFilesystems(app.ApplicationTag())
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("mariadb/0/0") // initial
	wc.AssertNoChange()

	app2, err := s.st.AddApplication(state.AddApplicationArgs{
		Name: "another", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: storage,
	})
	c.Assert(err, jc.ErrorIsNil)
	addUnit(app2)
	// no change, since we're only interested in the one application.
	wc.AssertNoChange()

	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	filesystemTag := names.NewFilesystemTag("mariadb/0/0")
	removeFilesystemStorageInstance(c, s.storageBackend, filesystemTag)

	err = s.storageBackend.DestroyFilesystem(filesystemTag, false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mariadb/0/0")
	wc.AssertNoChange()

	attachments, err := s.storageBackend.FilesystemAttachments(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		err := s.storageBackend.DetachFilesystem(a.Host(), filesystemTag)
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.RemoveFilesystemAttachment(a.Host(), filesystemTag, false)
		c.Assert(err, jc.ErrorIsNil)
	}
	wc.AssertChange("mariadb/0/0") // Dying -> Dead
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystem(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	// no more changes after seeing Dead
	wc.AssertNoChange()
}

func (s *FilesystemCAASModelSuite) TestWatchUnitFilesystemAttachments(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data":  {Count: 1, Size: 1024, Pool: "kubernetes"},
		"cache": {Count: 1, Size: 1024, Pool: "rootfs"},
	}
	app, err := s.st.AddApplication(state.AddApplicationArgs{
		Name: "mariadb", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: storage,
	})
	c.Assert(err, jc.ErrorIsNil)

	addUnit := func(app *state.Application) *state.Unit {
		var err error
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		return u
	}
	addUnit(app)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchUnitFilesystemAttachments(app.ApplicationTag())
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)

	wc.AssertChange("mariadb/0:mariadb/0/0") // initial
	wc.AssertNoChange()

	app2, err := s.st.AddApplication(state.AddApplicationArgs{
		Name: "another", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: storage,
	})
	c.Assert(err, jc.ErrorIsNil)
	addUnit(app2)
	// no change, since we're only interested in the one application.
	wc.AssertNoChange()

	err = s.storageBackend.DetachFilesystem(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	// no change, since we're only interested in attachments of
	// unit-scoped volumes.
	wc.AssertNoChange()

	removeFilesystemStorageInstance(c, s.storageBackend, names.NewFilesystemTag("mariadb/0/0"))
	err = s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("mariadb/0/0"), false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mariadb/0:mariadb/0/0") // dying
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystemAttachment(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("mariadb/0/0"), false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mariadb/0:mariadb/0/0") // removed
	wc.AssertNoChange()
}

func (s *FilesystemCAASModelSuite) TestAddExistingFilesystemVolumeBackedDuplicateVolumeId(c *gc.C) {
	// First, create a storage instance with a filesystem and set its VolumeId
	_, _, storageTag1 := s.setupSingleStorage(c, "filesystem", "kubernetes")

	volume := s.storageInstanceVolume(c, storageTag1)
	err := s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{
		Pool:     "kubernetes",
		Size:     123,
		VolumeId: "existing-volume-123",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Now try to add a filesystem with a backing volume that has the same VolumeId
	fsInfo := state.FilesystemInfo{
		Pool: "kubernetes",
		Size: 123,
	}
	volInfo2 := state.VolumeInfo{
		Pool:     "kubernetes",
		Size:     123,
		VolumeId: "existing-volume-123", // Same VolumeId as the first volume
	}
	_, err = s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo2, "fsdata")
	c.Assert(err, gc.ErrorMatches, `cannot add existing filesystem: volume with provider-id "existing-volume-123" exists, id: "0"`)
}

func (s *FilesystemCAASModelSuite) TestAddExistingFilesystemVolumeBackedUniqueVolumeId(c *gc.C) {
	// First, create a storage instance with a filesystem and set its VolumeId
	_, _, storageTag1 := s.setupSingleStorage(c, "filesystem", "kubernetes")

	volume := s.storageInstanceVolume(c, storageTag1)
	err := s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{
		Pool:     "kubernetes",
		Size:     123,
		VolumeId: "existing-volume-123",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Now try to add a filesystem with a backing volume that has a different VolumeId
	fsInfo := state.FilesystemInfo{
		Pool: "kubernetes",
		Size: 123,
	}
	volInfo2 := state.VolumeInfo{
		Pool:     "kubernetes",
		Size:     123,
		VolumeId: "different-volume-456", // Different VolumeId
	}
	storageTag2, err := s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo2, "fsdata")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageTag2, gc.Equals, names.NewStorageTag("fsdata/1"))

	// Verify both the filesystem and its backing volume were created
	filesystem, err := s.storageBackend.StorageInstanceFilesystem(storageTag2)
	c.Assert(err, jc.ErrorIsNil)
	fsInfoOut, err := filesystem.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fsInfoOut.FilesystemId, gc.Equals, "filesystem-1")
	c.Assert(fsInfoOut.Pool, gc.Equals, "kubernetes")
	c.Assert(fsInfoOut.Size, gc.Equals, uint64(123))

	backingVolume, err := s.storageBackend.StorageInstanceVolume(storageTag2)
	c.Assert(err, jc.ErrorIsNil)
	volInfoOut, err := backingVolume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volInfoOut.VolumeId, gc.Equals, "different-volume-456")
	c.Assert(volInfoOut.Pool, gc.Equals, "kubernetes")
	c.Assert(volInfoOut.Size, gc.Equals, uint64(123))
}

func (s *FilesystemStateSuite) TestParseFilesystemAttachmentId(c *gc.C) {
	assertValid := func(id string, m names.Tag, v names.FilesystemTag) {
		machineTag, filesystemTag, err := state.ParseFilesystemAttachmentId(id)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(machineTag, gc.Equals, m)
		c.Assert(filesystemTag, gc.Equals, v)
	}
	assertValid("0:0", names.NewMachineTag("0"), names.NewFilesystemTag("0"))
	assertValid("0:0/1", names.NewMachineTag("0"), names.NewFilesystemTag("0/1"))
	assertValid("0/lxd/0:1", names.NewMachineTag("0/lxd/0"), names.NewFilesystemTag("1"))
	assertValid("some-unit/0:1", names.NewUnitTag("some-unit/0"), names.NewFilesystemTag("1"))
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

func (s *FilesystemIAASModelSuite) TestRemoveStorageInstanceDestroysAndUnassignsFilesystem(c *gc.C) {
	filesystem, filesystemAttachment, storageAttachment := s.addUnitWithFilesystem(c, "modelscoped-block", true)
	volume := s.filesystemVolume(c, filesystemAttachment.Filesystem())
	storageTag := storageAttachment.StorageInstance()
	unitTag := storageAttachment.Unit()

	err := s.storageBackend.SetFilesystemAttachmentInfo(
		filesystemAttachment.Host().(names.MachineTag),
		filesystem.FilesystemTag(),
		state.FilesystemAttachmentInfo{},
	)
	c.Assert(err, jc.ErrorIsNil)

	u, err := s.st.Unit(unitTag.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(storageTag, unitTag, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance and attachment are dying, but not yet
	// removed from state. The filesystem should still be assigned.
	s.storageInstanceFilesystem(c, storageTag)
	s.storageInstanceVolume(c, storageTag)

	err = s.storageBackend.RemoveStorageAttachment(storageTag, unitTag, false)
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance is now gone; the filesystem should no longer
	// be assigned to the storage.
	_, err = s.storageBackend.StorageInstanceFilesystem(storageTag)
	c.Assert(err, gc.ErrorMatches, `filesystem for storage instance "data/0" not found`)
	_, err = s.storageBackend.StorageInstanceVolume(storageTag)
	c.Assert(err, gc.ErrorMatches, `volume for storage instance "data/0" not found`)

	// The filesystem and volume should still exist. The filesystem
	// should be dying; the volume will be destroyed only once the
	// filesystem is removed.
	f := s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(f.Life(), gc.Equals, state.Dying)
	v := s.volume(c, volume.VolumeTag())
	c.Assert(v.Life(), gc.Equals, state.Alive)
}

func (s *FilesystemIAASModelSuite) TestReleaseStorageInstanceFilesystemReleasing(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped")
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	c.Assert(filesystem.Releasing(), jc.IsFalse)
	err := s.storageBackend.SetFilesystemInfo(filesystem.FilesystemTag(), state.FilesystemInfo{FilesystemId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)

	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.ReleaseStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	// The filesystem should should be dying, and releasing.
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Dying)
	c.Assert(filesystem.Releasing(), jc.IsTrue)
}

func (s *FilesystemIAASModelSuite) TestReleaseStorageInstanceFilesystemUnreleasable(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped-unreleasable")
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	c.Assert(filesystem.Releasing(), jc.IsFalse)
	err := s.storageBackend.SetFilesystemInfo(filesystem.FilesystemTag(), state.FilesystemInfo{FilesystemId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)

	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.ReleaseStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, gc.ErrorMatches,
		`cannot release storage "data/0": storage provider "modelscoped-unreleasable" does not support releasing storage`)
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	// The filesystem should should be dying, and releasing.
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Alive)
	c.Assert(filesystem.Releasing(), jc.IsFalse)
}

func (s *FilesystemIAASModelSuite) TestSetFilesystemAttachmentInfoFilesystemNotProvisioned(c *gc.C) {
	_, filesystemAttachment, _ := s.addUnitWithFilesystemUnprovisioned(c, "rootfs", false)
	err := s.storageBackend.SetFilesystemAttachmentInfo(
		filesystemAttachment.Host().(names.MachineTag),
		filesystemAttachment.Filesystem(),
		state.FilesystemAttachmentInfo{},
	)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem attachment 0/0:0: filesystem "0/0" not provisioned`)
}

func (s *FilesystemIAASModelSuite) TestSetFilesystemAttachmentInfoMachineNotProvisioned(c *gc.C) {
	_, filesystemAttachment, _ := s.addUnitWithFilesystemUnprovisioned(c, "rootfs", false)
	err := s.storageBackend.SetFilesystemInfo(
		filesystemAttachment.Filesystem(),
		state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetFilesystemAttachmentInfo(
		filesystemAttachment.Host(),
		filesystemAttachment.Filesystem(),
		state.FilesystemAttachmentInfo{},
	)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem attachment 0/0:0: machine 0 not provisioned`)
}

func (s *FilesystemIAASModelSuite) TestSetFilesystemInfoVolumeAttachmentNotProvisioned(c *gc.C) {
	filesystem, _, _ := s.addUnitWithFilesystemUnprovisioned(c, "modelscoped-block", true)
	err := s.storageBackend.SetFilesystemInfo(
		filesystem.FilesystemTag(),
		state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"},
	)
	c.Assert(err, gc.ErrorMatches, `cannot set info for filesystem "0": backing volume "0" is not attached`)
}

func (s *FilesystemIAASModelSuite) TestDestroyFilesystem(c *gc.C) {
	filesystem, _ := s.setupFilesystemAttachment(c, "rootfs")
	assertDestroy := func() {
		s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	}
	defer state.SetBeforeHooks(c, s.st, assertDestroy).Check()
	assertDestroy()
}

func (s *FilesystemStateSuite) TestDestroyFilesystemNotFound(c *gc.C) {
	err := s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("0"), false)
	c.Assert(err, gc.ErrorMatches, `destroying filesystem 0: filesystem "0" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *FilesystemStateSuite) TestDestroyFilesystemStorageAssignedNoForce(c *gc.C) {
	// Create a filesystem-type storage instance, and show that we
	// cannot destroy the filesystem while there is storage assigned.
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	err := s.storageBackend.DestroyFilesystem(filesystem.FilesystemTag(), false)
	c.Assert(err, gc.ErrorMatches, "destroying filesystem .*0/0: filesystem is assigned to storage data/0")

	// We must destroy the unit before we can remove the storage.
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, s.storageBackend, storageTag)
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
}

func (s *FilesystemStateSuite) TestDestroyFilesystemStorageAssignedWithForce(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	err := s.storageBackend.DestroyFilesystem(filesystem.FilesystemTag(), true)
	c.Assert(err, jc.ErrorIsNil)
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Dying)
}

func (s *FilesystemIAASModelSuite) TestDestroyFilesystemNoAttachments(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")

	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.st, func() {
		err := s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
		c.Assert(err, jc.ErrorIsNil)
		assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
	}).Check()

	// There are no more attachments, so the filesystem should
	// be progressed directly to Dead.
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dead)
}

func (s *FilesystemIAASModelSuite) TestRemoveFilesystem(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	assertRemove := func() {
		err = s.storageBackend.RemoveFilesystem(filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		_, err = s.storageBackend.Filesystem(filesystem.FilesystemTag())
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
	defer state.SetBeforeHooks(c, s.st, assertRemove).Check()
	assertRemove()
}

func (s *FilesystemIAASModelSuite) TestRemoveFilesystemVolumeBacked(c *gc.C) {
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

	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	// Likewise for the volume attachment.
	assertVolumeAttachmentLife(state.Alive)

	err = s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	// Removing the filesystem attachment causes the backing-volume
	// to be detached.
	assertVolumeAttachmentLife(state.Dying)

	// Removing the last attachment should cause the filesystem
	// to be removed, since it is volume-backed and dying.
	_, err = s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	// Removing the filesystem causes the backing-volume to be
	// destroyed.
	assertVolumeLife(state.Dying)

	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
}

func (s *FilesystemIAASModelSuite) TestFilesystemVolumeBackedDestroyDetachVolumeFail(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped-block")
	volume := s.filesystemVolume(c, filesystem.FilesystemTag())

	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	// Can't destroy (detach) volume until the filesystem (attachment) is removed.
	err = s.storageBackend.DetachVolume(machine.MachineTag(), volume.VolumeTag(), false)
	c.Assert(err, gc.ErrorMatches, "detaching volume 0 from machine 0: volume contains attached filesystem")
	c.Assert(err, jc.Satisfies, state.IsContainsFilesystem)
	err = s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, gc.ErrorMatches, "destroying volume 0: volume contains filesystem")
	c.Assert(err, jc.Satisfies, state.IsContainsFilesystem)
	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())

	err = s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.storageBackend.DetachVolume(machine.MachineTag(), volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestRemoveFilesystemNotFound(c *gc.C) {
	err := s.storageBackend.RemoveFilesystem(names.NewFilesystemTag("42"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FilesystemIAASModelSuite) TestRemoveFilesystemNotDead(c *gc.C) {
	filesystem, _ := s.setupFilesystemAttachment(c, "rootfs")
	err := s.storageBackend.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, gc.ErrorMatches, "removing filesystem 0/0: filesystem is not dead")
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	err = s.storageBackend.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, gc.ErrorMatches, "removing filesystem 0/0: filesystem is not dead")
}

func (s *FilesystemIAASModelSuite) TestDetachFilesystem(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")
	assertDetach := func() {
		err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		attachment := s.filesystemAttachment(c, machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(attachment.Life(), gc.Equals, state.Dying)
	}
	defer state.SetBeforeHooks(c, s.st, assertDetach).Check()
	assertDetach()
}

func (s *FilesystemIAASModelSuite) TestRemoveLastFilesystemAttachment(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")

	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	// The filesystem has no attachments, so it should go straight to Dead.
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dead)
	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
}

func (s *FilesystemIAASModelSuite) TestRemoveLastFilesystemAttachmentConcurrently(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")

	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.st, func() {
		s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	}).Check()

	err = s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	// Last attachment was removed, and the filesystem was (concurrently)
	// destroyed, so the filesystem should be Dead.
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Dead)
	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
}

func (s *FilesystemStateSuite) TestRemoveFilesystemAttachmentNotFound(c *gc.C) {
	err := s.storageBackend.RemoveFilesystemAttachment(names.NewMachineTag("42"), names.NewFilesystemTag("42"), false)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `removing attachment of filesystem 42 from machine 42: filesystem "42" on "machine 42" not found`)
}

func (s *FilesystemIAASModelSuite) TestRemoveFilesystemAttachmentConcurrently(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")
	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	remove := func() {
		err := s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
		c.Assert(err, jc.ErrorIsNil)
		assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
	}
	defer state.SetBeforeHooks(c, s.st, remove).Check()
	remove()
}

func (s *FilesystemIAASModelSuite) TestRemoveFilesystemAttachmentAlive(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	err := s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, gc.ErrorMatches, "removing attachment of filesystem 0/0 from machine 0: filesystem attachment is not dying")
}

func (s *FilesystemIAASModelSuite) TestRemoveMachineRemovesFilesystems(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")

	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	c.Assert(machine.EnsureDead(), jc.ErrorIsNil)
	c.Assert(machine.Remove(), jc.ErrorIsNil)

	// Machine is gone: filesystem should be gone too.
	_, err := s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	attachments, err := s.storageBackend.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 0)
}

func (s *FilesystemIAASModelSuite) TestDestroyMachineRemovesNonDetachableFilesystems(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "loop")

	// Destroy the machine and run cleanups, which should cause the
	// non-detachable filesystems to be destroyed, detached, and
	// finally removed.
	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	assertCleanupRuns(c, s.st)

	_, err := s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *FilesystemIAASModelSuite) TestDestroyMachineDetachesDetachableFilesystems(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped-block")

	// Destroy the machine and run cleanups, which should cause the
	// detachable filesystems to be detached, but not destroyed.
	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	assertCleanupRuns(c, s.st)
	s.testfilesystemDetached(
		c, machine.MachineTag(), filesystem.FilesystemTag(),
	)
}

// TODO(caas) - destroy caas storage when unit dies
func (s *FilesystemIAASModelSuite) TestDestroyHostDetachesDetachableFilesystems(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped-block")
	hostTag := s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	// Destroying the unit should, if necessary, destroy its host machine, which
	// triggers the detachment of storage.
	s.obliterateUnit(c, u.UnitTag())
	assertCleanupRuns(c, s.st)

	s.testfilesystemDetached(
		c, hostTag, filesystem.FilesystemTag(),
	)
}

func (s *FilesystemStateSuite) testfilesystemDetached(
	c *gc.C,
	hostTag names.Tag,
	filesystemTag names.FilesystemTag,
) {
	// Filesystem is still alive...
	filesystem, err := s.storageBackend.Filesystem(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystem.Life(), gc.Equals, state.Alive)

	// ... but it has been detached.
	_, err = s.storageBackend.FilesystemAttachment(hostTag, filesystemTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	filesystemStatus, err := filesystem.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemStatus.Status, gc.Equals, status.Detached)
	c.Assert(filesystemStatus.Message, gc.Equals, "")
}

func (s *FilesystemIAASModelSuite) TestDestroyManualMachineDoesntRemoveNonDetachableFilesystems(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "loop")

	// Make this a manual machine, so the cleanup.
	err := machine.SetProvisioned("inst-id", "", "manual:machine", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy the machine and run cleanups, which should cause the
	// non-detachable filesystems and attachments to be set to Dying,
	// but not completely removed.
	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	assertCleanupRuns(c, s.st)

	filesystem, err = s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystem.Life(), gc.Equals, state.Dying)
	attachment, err := s.storageBackend.FilesystemAttachment(
		machine.MachineTag(),
		filesystem.FilesystemTag(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachment.Life(), gc.Equals, state.Dying)
}

func (s *FilesystemIAASModelSuite) TestDestroyManualMachineDoesntDetachDetachableFilesystems(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped-block")

	// Make this a manual machine, so the cleanup.
	err := machine.SetProvisioned("inst-id", "", "manual:machine", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy the machine and run cleanups, which should cause the
	// detachable filesystem attachments to be set to Dying, but not
	// completely removed. The filesystem itself should be left Alive.
	c.Assert(machine.Destroy(), jc.ErrorIsNil)
	assertCleanupRuns(c, s.st)

	filesystem, err = s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystem.Life(), gc.Equals, state.Alive)
	attachment, err := s.storageBackend.FilesystemAttachment(
		machine.MachineTag(),
		filesystem.FilesystemTag(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachment.Life(), gc.Equals, state.Dying)
}

func (s *FilesystemIAASModelSuite) TestFilesystemMachineScoped(c *gc.C) {
	// Machine-scoped filesystems created unassigned to a storage
	// instance are bound to the machine.
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")

	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, gc.ErrorMatches, "detaching filesystem 0/0 from machine 0: filesystem is not detachable")
	err = machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.storageBackend.FilesystemAttachment(
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
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	// The filesystem should transition to Dying when the storage is removed.
	// We must destroy the unit before we can remove the storage.
	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, s.storageBackend, storageTag)
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), gc.Equals, state.Dying)
}

func (s *FilesystemIAASModelSuite) TestEnsureMachineDeadAddFilesystemConcurrently(c *gc.C) {
	_, machine := s.setupFilesystemAttachment(c, "rootfs")
	addFilesystem := func() {
		_, u, _ := s.setupSingleStorage(c, "filesystem", "rootfs")
		err := u.AssignToMachine(machine)
		c.Assert(err, jc.ErrorIsNil)
		s.obliterateUnit(c, u.UnitTag())
	}
	defer state.SetBeforeHooks(c, s.st, addFilesystem).Check()

	// Adding another filesystem to the machine will cause EnsureDead to
	// retry, but it will succeed because both filesystems are inherently
	// machine-bound.
	err := machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FilesystemIAASModelSuite) TestEnsureMachineDeadRemoveFilesystemConcurrently(c *gc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	removeFilesystem := func() {
		s.obliterateFilesystem(c, filesystem.FilesystemTag())
	}
	defer state.SetBeforeHooks(c, s.st, removeFilesystem).Check()

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
	ch := s.createStorageCharmWithSeries(c, "storage-filesystem", charm.Storage{
		Name:     "data",
		Type:     charm.StorageFilesystem,
		CountMin: countMin,
		CountMax: countMax,
		Location: location,
	}, s.series)
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("rootfs", 1024, 1),
	}

	app := s.AddTestingApplicationWithStorage(c, "storage-filesystem", ch, storage)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	hostTag := s.maybeAssignUnit(c, unit)

	storageTag := names.NewStorageTag("data/0")
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemAttachment := s.filesystemAttachment(
		c, hostTag, filesystem.FilesystemTag(),
	)
	params, ok := filesystemAttachment.Params()
	c.Assert(ok, jc.IsTrue)
	c.Assert(params, jc.DeepEquals, expect)
}

func (s *FilesystemIAASModelSuite) TestFilesystemAttachmentParamsLocationConflictConcurrent(c *gc.C) {
	s.testFilesystemAttachmentParamsConcurrent(
		c, "/srv", "/srv",
		`cannot assign unit "storage-filesystem-after/0" to machine 0: `+
			`validating filesystem mount points: `+
			`mount point "/srv" for "data" storage contains mount point "/srv" for "data" storage`)
}

func (s *FilesystemIAASModelSuite) TestFilesystemAttachmentParamsLocationAutoConcurrent(c *gc.C) {
	s.testFilesystemAttachmentParamsConcurrent(c, "", "", "")
}

func (s *FilesystemIAASModelSuite) TestFilesystemAttachmentParamsLocationAutoAndManualConcurrent(c *gc.C) {
	s.testFilesystemAttachmentParamsConcurrent(c, "", "/srv", "")
}

func (s *FilesystemStateSuite) testFilesystemAttachmentParamsConcurrent(c *gc.C, locBefore, locAfter, expectErr string) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
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

	defer state.SetBeforeHooks(c, s.st, func() {
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

func (s *FilesystemIAASModelSuite) TestFilesystemAttachmentParamsConcurrentRemove(c *gc.C) {
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

	defer state.SetBeforeHooks(c, s.st, func() {
		err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.RemoveFilesystemAttachment(
			machine.MachineTag(), filesystem.FilesystemTag(), false,
		)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsLocationStorageDir(c *gc.C) {
	ch := s.createStorageCharmWithSeries(c, "storage-filesystem", charm.Storage{
		Name:     "data",
		Type:     charm.StorageFilesystem,
		CountMin: 1,
		CountMax: 1,
		Location: "/var/lib/juju/storage",
	}, s.series)
	app := s.AddTestingApplication(c, "storage-filesystem", ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	if s.series != "kubernetes" {
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	}
	c.Assert(err, gc.ErrorMatches, `.*`+
		`getting filesystem mount point for storage data: `+
		`invalid location "/var/lib/juju/storage": `+
		`must not fall within "/var/lib/juju/storage"`)
}

func (s *FilesystemIAASModelSuite) TestFilesystemAttachmentLocationConflict(c *gc.C) {
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

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystem(c *gc.C) {
	fsInfoIn := state.FilesystemInfo{
		Pool:         "modelscoped",
		Size:         123,
		FilesystemId: "foo",
	}
	storageTag, err := s.storageBackend.AddExistingFilesystem(fsInfoIn, nil, "pgdata")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageTag, gc.Equals, names.NewStorageTag("pgdata/0"))

	filesystem, err := s.storageBackend.StorageInstanceFilesystem(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	fsInfoOut, err := filesystem.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fsInfoOut, jc.DeepEquals, fsInfoIn)

	fsStatus, err := filesystem.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fsStatus.Status, gc.Equals, status.Detached)
}

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystemEmptyFilesystemId(c *gc.C) {
	fsInfoIn := state.FilesystemInfo{
		Pool: "modelscoped",
		Size: 123,
	}
	_, err := s.storageBackend.AddExistingFilesystem(fsInfoIn, nil, "pgdata")
	c.Assert(err, gc.ErrorMatches, "cannot add existing filesystem: empty filesystem ID not valid")
}

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystemVolumeBacked(c *gc.C) {
	fsInfoIn := state.FilesystemInfo{
		Pool: "modelscoped-block",
		Size: 123,
	}
	volInfoIn := state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "foo",
	}
	storageTag, err := s.storageBackend.AddExistingFilesystem(fsInfoIn, &volInfoIn, "pgdata")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageTag, gc.Equals, names.NewStorageTag("pgdata/0"))

	filesystem, err := s.storageBackend.StorageInstanceFilesystem(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	fsInfoOut, err := filesystem.Info()
	c.Assert(err, jc.ErrorIsNil)
	fsInfoIn.FilesystemId = "filesystem-0" // set by AddExistingFilesystem
	c.Assert(fsInfoOut, jc.DeepEquals, fsInfoIn)

	fsStatus, err := filesystem.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fsStatus.Status, gc.Equals, status.Detached)

	volume, err := s.storageBackend.StorageInstanceVolume(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	volInfoOut, err := volume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volInfoOut, jc.DeepEquals, volInfoIn)

	volStatus, err := volume.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volStatus.Status, gc.Equals, status.Detached)
}

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystemVolumeBackedVolumeInfoMissing(c *gc.C) {
	fsInfo := state.FilesystemInfo{
		Pool:         "modelscoped-block",
		Size:         123,
		FilesystemId: "foo",
	}
	_, err := s.storageBackend.AddExistingFilesystem(fsInfo, nil, "pgdata")
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
	_, err := s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo, "pgdata")
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
	_, err := s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo, "pgdata")
	c.Assert(err, gc.ErrorMatches, "cannot add existing filesystem: empty backing volume ID not valid")
}

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystemVolumeBackedDuplicateVolumeId(c *gc.C) {
	// First, create a storage instance with a block device and set its VolumeId
	_, u, storageTag1 := s.setupSingleStorage(c, "block", "modelscoped-block")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	volume := s.storageInstanceVolume(c, storageTag1)
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "existing-volume-123",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Now try to add a filesystem with a backing volume that has the same VolumeId
	fsInfo := state.FilesystemInfo{
		Pool: "modelscoped-block",
		Size: 123,
	}
	volInfo2 := state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "existing-volume-123", // Same VolumeId as the first volume
	}
	_, err = s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo2, "fsdata")
	c.Assert(err, gc.ErrorMatches, `cannot add existing filesystem: volume with provider-id "existing-volume-123" exists, id: "0"`)
}

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystemVolumeBackedUniqueVolumeId(c *gc.C) {
	// First, create a storage instance with a block device and set its VolumeId
	_, u, storageTag1 := s.setupSingleStorage(c, "block", "modelscoped-block")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	volume := s.storageInstanceVolume(c, storageTag1)
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "existing-volume-123",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Now try to add a filesystem with a backing volume that has a different VolumeId
	fsInfo := state.FilesystemInfo{
		Pool: "modelscoped-block",
		Size: 123,
	}
	volInfo2 := state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "different-volume-456", // Different VolumeId
	}
	storageTag2, err := s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo2, "fsdata")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageTag2, gc.Equals, names.NewStorageTag("fsdata/1"))

	// Verify both the filesystem and its backing volume were created
	filesystem, err := s.storageBackend.StorageInstanceFilesystem(storageTag2)
	c.Assert(err, jc.ErrorIsNil)
	fsInfoOut, err := filesystem.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fsInfoOut.FilesystemId, gc.Equals, "filesystem-0")
	c.Assert(fsInfoOut.Pool, gc.Equals, "modelscoped-block")
	c.Assert(fsInfoOut.Size, gc.Equals, uint64(123))

	backingVolume, err := s.storageBackend.StorageInstanceVolume(storageTag2)
	c.Assert(err, jc.ErrorIsNil)
	volInfoOut, err := backingVolume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volInfoOut.VolumeId, gc.Equals, "different-volume-456")
	c.Assert(volInfoOut.Pool, gc.Equals, "modelscoped-block")
	c.Assert(volInfoOut.Size, gc.Equals, uint64(123))
}

func (s *FilesystemStateSuite) setupFilesystemAttachment(c *gc.C, pool string) (state.Filesystem, *state.Machine) {
	machine, err := s.st.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{Pool: pool, Size: 1024},
			Attachment: state.FilesystemAttachmentParams{
				Location: "/srv",
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	sb, err := state.NewStorageBackend(s.st)
	c.Assert(err, jc.ErrorIsNil)
	attachments, err := sb.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 1)
	c.Assert(err, jc.ErrorIsNil)
	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
	return s.filesystem(c, attachments[0].Filesystem()), machine
}

func removeFilesystemStorageInstance(c *gc.C, sb *state.StorageBackend, filesystemTag names.FilesystemTag) {
	filesystem, err := sb.Filesystem(filesystemTag)
	c.Assert(err, jc.ErrorIsNil)
	storageTag, err := filesystem.Storage()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, sb, storageTag)
}

func (s *FilesystemStateSuite) assertDestroyFilesystem(c *gc.C, tag names.FilesystemTag, life state.Life) {
	err := s.storageBackend.DestroyFilesystem(tag, false)
	c.Assert(err, jc.ErrorIsNil)
	filesystem := s.filesystem(c, tag)
	c.Assert(filesystem.Life(), gc.Equals, life)
}
