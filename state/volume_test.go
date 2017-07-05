// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
)

type VolumeStateSuite struct {
	StorageStateSuiteBase
}

var _ = gc.Suite(&VolumeStateSuite{})

func (s *VolumeStateSuite) TestAddMachine(c *gc.C) {
	_, unit, _ := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineVolume(c, unit)
}

func (s *VolumeStateSuite) TestAssignToMachine(c *gc.C) {
	_, unit, _ := s.setupSingleStorage(c, "block", "loop-pool")
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineVolume(c, unit)
}

func (s *VolumeStateSuite) assertMachineVolume(c *gc.C, unit *state.Unit) {
	assignedMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	storageAttachments, err := s.State.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)
	storageInstance, err := s.State.StorageInstance(storageAttachments[0].StorageInstance())
	c.Assert(err, jc.ErrorIsNil)

	volume := s.storageInstanceVolume(c, storageInstance.StorageTag())
	c.Assert(volume.VolumeTag(), gc.Equals, names.NewVolumeTag("0/0"))
	volumeStorageTag, err := volume.StorageInstance()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeStorageTag, gc.Equals, storageInstance.StorageTag())
	_, err = volume.Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok := volume.Params()
	c.Assert(ok, jc.IsTrue)

	machine, err := s.State.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	volumeAttachments, err := im.MachineVolumeAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)
	c.Assert(volumeAttachments[0].Volume(), gc.Equals, volume.VolumeTag())
	c.Assert(volumeAttachments[0].Machine(), gc.Equals, machine.MachineTag())
	_, err = volumeAttachments[0].Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok = volumeAttachments[0].Params()
	c.Assert(ok, jc.IsTrue)

	_, err = im.VolumeAttachment(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)

	assertMachineStorageRefs(c, s.State, machine.MachineTag())
}

func (s *VolumeStateSuite) TestAddServiceInvalidPool(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("invalid-pool", 1024, 1),
	}
	_, err := s.State.AddApplication(state.AddApplicationArgs{Name: "storage-block", Charm: ch, Storage: storage})
	c.Assert(err, gc.ErrorMatches, `.* pool "invalid-pool" not found`)
}

func (s *VolumeStateSuite) TestAddServiceNoUserDefaultPool(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}
	app, err := s.State.AddApplication(state.AddApplicationArgs{Name: "storage-block", Charm: ch, Storage: storage})
	c.Assert(err, jc.ErrorIsNil)
	cons, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": state.StorageConstraints{
			Pool:  "loop",
			Size:  1024,
			Count: 1,
		},
		"allecto": state.StorageConstraints{
			Pool:  "loop",
			Size:  1024,
			Count: 0,
		},
	})
}

func (s *VolumeStateSuite) TestAddServiceDefaultPool(c *gc.C) {
	// Register a default pool.
	pm := poolmanager.New(state.NewStateSettings(s.State), storage.ChainedProviderRegistry{
		dummy.StorageProviders(),
		provider.CommonStorageProviders(),
	})
	_, err := pm.Create("default-block", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.UpdateModelConfig(map[string]interface{}{
		"storage-default-block-source": "default-block",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}
	app := s.AddTestingServiceWithStorage(c, "storage-block", ch, storage)
	cons, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": state.StorageConstraints{
			Pool:  "default-block",
			Size:  1024,
			Count: 1,
		},
		"allecto": state.StorageConstraints{
			Pool:  "loop",
			Size:  1024,
			Count: 0,
		},
	})
}

func (s *VolumeStateSuite) TestSetVolumeInfo(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	volume := s.storageInstanceVolume(c, storageTag)
	volumeTag := volume.VolumeTag()
	s.assertVolumeUnprovisioned(c, volumeTag)

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	volumeInfoSet := state.VolumeInfo{Size: 123, Persistent: true, VolumeId: "vol-ume"}
	err = im.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)
	volumeInfoSet.Pool = "loop-pool" // taken from params
	s.assertVolumeInfo(c, volumeTag, volumeInfoSet)
}

func (s *VolumeStateSuite) TestSetVolumeInfoNoVolumeId(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	volume := s.storageInstanceVolume(c, storageTag)
	volumeTag := volume.VolumeTag()
	s.assertVolumeUnprovisioned(c, volumeTag)

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	volumeInfoSet := state.VolumeInfo{Size: 123, Persistent: true}
	err = im.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
	c.Assert(err, gc.ErrorMatches, `cannot set info for volume "0/0": volume ID not set`)
}

func (s *VolumeStateSuite) TestSetVolumeInfoNoStorageAssigned(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	cons := constraints.MustParse("mem=4G")
	hc := instance.MustParseHardware("mem=2G")

	volumeParams := state.VolumeParams{
		Pool: "loop-pool",
		Size: 123,
	}
	machineTemplate := state.MachineTemplate{
		Series:                  "precise",
		Constraints:             cons,
		HardwareCharacteristics: hc,
		InstanceId:              "inst-id",
		Nonce:                   "nonce",
		Jobs:                    oneJob,
		Volumes: []state.MachineVolumeParams{{
			Volume: volumeParams,
		}},
	}
	machines, err := s.State.AddMachines(machineTemplate)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	m, err := s.State.Machine(machines[0].Id())
	c.Assert(err, jc.ErrorIsNil)

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	volumeAttachments, err := im.MachineVolumeAttachments(m.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)
	volumeTag := volumeAttachments[0].Volume()

	volume := s.volume(c, volumeTag)
	_, err = volume.StorageInstance()
	c.Assert(err, jc.Satisfies, errors.IsNotAssigned)

	s.assertVolumeUnprovisioned(c, volumeTag)
	volumeInfoSet := state.VolumeInfo{Size: 123, VolumeId: "vol-ume"}
	err = im.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)
	volumeInfoSet.Pool = "loop-pool" // taken from params
	s.assertVolumeInfo(c, volumeTag, volumeInfoSet)
}

func (s *VolumeStateSuite) TestSetVolumeInfoImmutable(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	volume := s.storageInstanceVolume(c, storageTag)
	volumeTag := volume.VolumeTag()

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	volumeInfoSet := state.VolumeInfo{Size: 123, VolumeId: "vol-ume"}
	err = im.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	// The first call to SetVolumeInfo takes the pool name from
	// the params; the second does not, but it must not change
	// either. Callers are expected to get the existing info and
	// update it, leaving immutable values intact.
	err = im.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
	c.Assert(err, gc.ErrorMatches, `cannot set info for volume "0/0": cannot change pool from "loop-pool" to ""`)

	volumeInfoSet.Pool = "loop-pool"
	s.assertVolumeInfo(c, volumeTag, volumeInfoSet)
}

func (s *VolumeStateSuite) TestWatchVolumeAttachment(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(assignedMachineId)

	volume := s.storageInstanceVolume(c, storageTag)
	volumeTag := volume.VolumeTag()

	w := s.State.WatchVolumeAttachment(machineTag, volumeTag)
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	machine, err := s.State.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	// volume attachment will NOT react to volume changes
	err = im.SetVolumeInfo(volumeTag, state.VolumeInfo{VolumeId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = im.SetVolumeAttachmentInfo(
		machineTag, volumeTag, state.VolumeAttachmentInfo{
			DeviceName: "xvdf1",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *VolumeStateSuite) TestWatchModelVolumes(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "block")
	addUnit := func() {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	addUnit()

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	w := s.State.WatchModelVolumes()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0", "1") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChangeInSingleEvent("4", "5")
	wc.AssertNoChange()

	volume, err := im.Volume(names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	storageTag, err := volume.StorageInstance()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, s.State, storageTag)
	err = im.DestroyVolume(names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0") // dying
	wc.AssertNoChange()

	err = im.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)

	err = im.RemoveVolume(names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0") // removed
	wc.AssertNoChange()
}

func (s *VolumeStateSuite) TestWatchEnvironVolumeAttachments(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "block")
	addUnit := func() {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	addUnit()

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	w := s.State.WatchModelVolumeAttachments()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0:0", "0:1") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChangeInSingleEvent("1:4", "1:5")
	wc.AssertNoChange()

	err = im.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0:0") // dying
	wc.AssertNoChange()

	err = im.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0:0") // removed
	wc.AssertNoChange()
}

func (s *VolumeStateSuite) TestWatchMachineVolumes(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "block", "machinescoped", "modelscoped")
	addUnit := func() {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	addUnit()

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	w := s.State.WatchMachineVolumes(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0/0", "0/1") // initial
	wc.AssertNoChange()

	addUnit()
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	volume, err := im.Volume(names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	storageTag, err := volume.StorageInstance()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, s.State, storageTag)
	err = im.DestroyVolume(volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0/0") // dying
	wc.AssertNoChange()

	err = im.DestroyVolume(names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolume(names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0/0") // removed
	wc.AssertNoChange()
}

func (s *VolumeStateSuite) TestWatchMachineVolumeAttachments(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "block", "machinescoped", "modelscoped")
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

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	w := s.State.WatchMachineVolumeAttachments(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0:0/0", "0:0/1") // initial
	wc.AssertNoChange()

	addUnit(nil)
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	err = im.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("2"))
	c.Assert(err, jc.ErrorIsNil)
	// no change, since we're only interested in attachments of
	// machine-scoped volumes.
	wc.AssertNoChange()

	removeVolumeStorageInstance(c, s.State, names.NewVolumeTag("0/0"))
	err = im.DestroyVolume(names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0:0/0") // dying
	wc.AssertNoChange()

	err = im.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("0:0/0") // removed
	wc.AssertNoChange()

	addUnit(m0)
	wc.AssertChangeInSingleEvent("0:0/8", "0:0/9") // added
}

func (s *VolumeStateSuite) TestParseVolumeAttachmentId(c *gc.C) {
	assertValid := func(id string, m names.MachineTag, v names.VolumeTag) {
		machineTag, volumeTag, err := state.ParseVolumeAttachmentId(id)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(machineTag, gc.Equals, m)
		c.Assert(volumeTag, gc.Equals, v)
	}
	assertValid("0:0", names.NewMachineTag("0"), names.NewVolumeTag("0"))
	assertValid("0:0/1", names.NewMachineTag("0"), names.NewVolumeTag("0/1"))
	assertValid("0/lxd/0:1", names.NewMachineTag("0/lxd/0"), names.NewVolumeTag("1"))
}

func (s *VolumeStateSuite) TestParseVolumeAttachmentIdError(c *gc.C) {
	assertError := func(id, expect string) {
		_, _, err := state.ParseVolumeAttachmentId(id)
		c.Assert(err, gc.ErrorMatches, expect)
	}
	assertError("", `invalid volume attachment ID ""`)
	assertError("0", `invalid volume attachment ID "0"`)
	assertError("0:foo", `invalid volume attachment ID "0:foo"`)
	assertError("bar:0", `invalid volume attachment ID "bar:0"`)
}

func (s *VolumeStateSuite) TestAllVolumes(c *gc.C) {
	_, expected, _ := s.assertCreateVolumes(c)

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	volumes, err := im.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	tags := make([]names.VolumeTag, len(volumes))
	for i, v := range volumes {
		tags[i] = v.VolumeTag()
	}
	c.Assert(tags, jc.SameContents, expected)
}

func (s *VolumeStateSuite) assertCreateVolumes(c *gc.C) (_ *state.Machine, all, persistent []names.VolumeTag) {
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.MachineVolumeParams{{
			Volume: state.VolumeParams{Pool: "persistent-block", Size: 1024},
		}, {
			Volume: state.VolumeParams{Pool: "loop-pool", Size: 2048},
		}, {
			Volume: state.VolumeParams{Pool: "static", Size: 2048},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertMachineStorageRefs(c, s.State, machine.MachineTag())

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	volume1 := s.volume(c, names.NewVolumeTag("0"))
	volume2 := s.volume(c, names.NewVolumeTag("0/1"))
	volume3 := s.volume(c, names.NewVolumeTag("2"))

	volumeInfoSet := state.VolumeInfo{Size: 123, Persistent: true, VolumeId: "vol-1"}
	err = im.SetVolumeInfo(volume1.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	volumeInfoSet = state.VolumeInfo{Size: 456, Persistent: false, VolumeId: "vol-2"}
	err = im.SetVolumeInfo(volume2.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	all = []names.VolumeTag{
		volume1.VolumeTag(),
		volume2.VolumeTag(),
		volume3.VolumeTag(),
	}
	persistent = []names.VolumeTag{
		volume1.VolumeTag(),
	}
	return machine, all, persistent
}

func (s *VolumeStateSuite) TestRemoveStorageInstanceDestroysAndUnassignsVolume(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "modelscoped")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	volume := s.storageInstanceVolume(c, storageTag)
	c.Assert(err, jc.ErrorIsNil)

	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	// Provision volume attachment so that detaching the storage
	// attachment does not short-circuit.
	defer state.SetBeforeHooks(c, s.State, func() {
		machine := unitMachine(c, s.State, u)
		err = machine.SetProvisioned("inst-id", "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
		err = im.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{VolumeId: "vol-123"})
		c.Assert(err, jc.ErrorIsNil)
		err = im.SetVolumeAttachmentInfo(
			machine.MachineTag(), volume.VolumeTag(),
			state.VolumeAttachmentInfo{DeviceName: "xvdf1"},
		)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance and attachment are dying, but not yet
	// removed from state. The volume should still be assigned.
	s.storageInstanceVolume(c, storageTag)

	err = s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance is now gone; the volume should no longer
	// be assigned to the storage.
	_, err = im.StorageInstanceVolume(storageTag)
	c.Assert(err, gc.ErrorMatches, `volume for storage instance "data/0" not found`)

	// The volume should still exist, but it should be dying.
	v := s.volume(c, volume.VolumeTag())
	c.Assert(v.Life(), gc.Equals, state.Dying)
}

func (s *VolumeStateSuite) TestSetVolumeAttachmentInfoVolumeNotProvisioned(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(assignedMachineId)

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	volume := s.storageInstanceVolume(c, storageTag)
	volumeTag := volume.VolumeTag()

	err = im.SetVolumeAttachmentInfo(
		machineTag, volumeTag, state.VolumeAttachmentInfo{
			DeviceName: "xvdf1",
		},
	)
	c.Assert(err, gc.ErrorMatches, `cannot set info for volume attachment 0/0:0: volume "0/0" not provisioned`)
}

func (s *VolumeStateSuite) TestDestroyVolume(c *gc.C) {
	volume, _ := s.setupMachineScopedVolumeAttachment(c)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	assertDestroy := func() {
		err = im.DestroyVolume(volume.VolumeTag())
		c.Assert(err, jc.ErrorIsNil)
		volume = s.volume(c, volume.VolumeTag())
		c.Assert(volume.Life(), gc.Equals, state.Dying)
	}
	defer state.SetBeforeHooks(c, s.State, assertDestroy).Check()
	assertDestroy()
}

func (s *VolumeStateSuite) TestDestroyVolumeNotFound(c *gc.C) {
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = im.DestroyVolume(names.NewVolumeTag("0"))
	c.Assert(err, gc.ErrorMatches, `destroying volume 0: volume "0" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *VolumeStateSuite) TestDestroyVolumeStorageAssigned(c *gc.C) {
	volume, _, u := s.setupStorageVolumeAttachment(c)
	storageTag, err := volume.StorageInstance()
	c.Assert(err, jc.ErrorIsNil)

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	err = im.DestroyVolume(volume.VolumeTag())
	c.Assert(err, gc.ErrorMatches, "destroying volume 0: volume is assigned to storage data/0")

	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, s.State, storageTag)
	err = im.DestroyVolume(volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *VolumeStateSuite) TestDestroyVolumeNoAttachments(c *gc.C) {
	volume, machine := s.setupModelScopedVolumeAttachment(c)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	err = im.DetachVolume(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := im.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag())
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = im.DestroyVolume(volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	volume = s.volume(c, volume.VolumeTag())

	// There are no more attachments, so the volume should
	// have been progressed directly to Dead.
	c.Assert(volume.Life(), gc.Equals, state.Dead)
}

func (s *VolumeStateSuite) TestRemoveVolume(c *gc.C) {
	volume, machine := s.setupMachineScopedVolumeAttachment(c)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	err = im.DestroyVolume(volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	assertRemove := func() {
		err = im.RemoveVolume(volume.VolumeTag())
		c.Assert(err, jc.ErrorIsNil)
		_, err = im.Volume(volume.VolumeTag())
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
	defer state.SetBeforeHooks(c, s.State, assertRemove).Check()
	assertRemove()
}

func (s *VolumeStateSuite) TestRemoveVolumeNotFound(c *gc.C) {
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolume(names.NewVolumeTag("42"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *VolumeStateSuite) TestRemoveVolumeNotDead(c *gc.C) {
	volume, _ := s.setupMachineScopedVolumeAttachment(c)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolume(volume.VolumeTag())
	c.Assert(err, gc.ErrorMatches, "removing volume 0/0: volume is not dead")
	err = im.DestroyVolume(volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolume(volume.VolumeTag())
	c.Assert(err, gc.ErrorMatches, "removing volume 0/0: volume is not dead")
}

func (s *VolumeStateSuite) TestDetachVolume(c *gc.C) {
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	volume, machine := s.setupModelScopedVolumeAttachment(c)
	assertDetach := func() {
		err := im.DetachVolume(machine.MachineTag(), volume.VolumeTag())
		c.Assert(err, jc.ErrorIsNil)
		attachment := s.volumeAttachment(c, machine.MachineTag(), volume.VolumeTag())
		c.Assert(attachment.Life(), gc.Equals, state.Dying)
	}
	defer state.SetBeforeHooks(c, s.State, assertDetach).Check()
	assertDetach()
}

func (s *VolumeStateSuite) TestRemoveLastVolumeAttachment(c *gc.C) {
	volume, machine := s.setupModelScopedVolumeAttachment(c)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	err = im.DetachVolume(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)

	err = im.DestroyVolume(volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	volume = s.volume(c, volume.VolumeTag())
	c.Assert(volume.Life(), gc.Equals, state.Dying)

	err = im.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)

	// The volume was Dying when the last attachment was
	// removed, so the volume should now be Dead.
	volume = s.volume(c, volume.VolumeTag())
	c.Assert(volume.Life(), gc.Equals, state.Dead)
}

func (s *VolumeStateSuite) TestRemoveLastVolumeAttachmentConcurrently(c *gc.C) {
	volume, machine := s.setupModelScopedVolumeAttachment(c)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	err = im.DetachVolume(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := im.DestroyVolume(volume.VolumeTag())
		c.Assert(err, jc.ErrorIsNil)
		volume := s.volume(c, volume.VolumeTag())
		c.Assert(volume.Life(), gc.Equals, state.Dying)
	}).Check()

	err = im.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)

	// Last attachment was removed, and the volume was (concurrently)
	// destroyed, so the volume should be Dead.
	volume = s.volume(c, volume.VolumeTag())
	c.Assert(volume.Life(), gc.Equals, state.Dead)
}

func (s *VolumeStateSuite) TestRemoveVolumeAttachmentNotFound(c *gc.C) {
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolumeAttachment(names.NewMachineTag("42"), names.NewVolumeTag("42"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `removing attachment of volume 42 from machine 42: volume "42" on machine "42" not found`)
}

func (s *VolumeStateSuite) TestRemoveVolumeAttachmentConcurrently(c *gc.C) {
	volume, machine := s.setupMachineScopedVolumeAttachment(c)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = im.DestroyVolume(volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	remove := func() {
		err := im.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag())
		c.Assert(err, jc.ErrorIsNil)
		assertMachineStorageRefs(c, s.State, machine.MachineTag())
	}
	defer state.SetBeforeHooks(c, s.State, remove).Check()
	remove()
}

func (s *VolumeStateSuite) TestRemoveVolumeAttachmentAlive(c *gc.C) {
	volume, machine := s.setupMachineScopedVolumeAttachment(c)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, gc.ErrorMatches, "removing attachment of volume 0/0 from machine 0: volume attachment is not dying")
}

func (s *VolumeStateSuite) TestRemoveMachineRemovesVolumes(c *gc.C) {
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.MachineVolumeParams{{
			Volume: state.VolumeParams{Pool: "persistent-block", Size: 1024}, // unprovisioned
		}, {
			Volume: state.VolumeParams{Pool: "loop-pool", Size: 2048}, // provisioned
		}, {
			Volume: state.VolumeParams{Pool: "loop-pool", Size: 2048}, // unprovisioned
		}, {
			Volume: state.VolumeParams{Pool: "loop-pool", Size: 2048}, // provisioned, non-persistent
		}, {
			Volume: state.VolumeParams{Pool: "static", Size: 2048}, // provisioned
		}, {
			Volume: state.VolumeParams{Pool: "static", Size: 2048}, // unprovisioned
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	volumeInfoSet := state.VolumeInfo{Size: 123, Persistent: true, VolumeId: "vol-1"}
	err = im.SetVolumeInfo(names.NewVolumeTag("0/1"), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)
	volumeInfoSet = state.VolumeInfo{Size: 456, Persistent: false, VolumeId: "vol-2"}
	err = im.SetVolumeInfo(names.NewVolumeTag("0/3"), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)
	volumeInfoSet = state.VolumeInfo{Size: 789, Persistent: false, VolumeId: "vol-3"}
	err = im.SetVolumeInfo(names.NewVolumeTag("4"), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	allVolumes, err := im.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)

	persistentVolumes := make([]state.Volume, 0, len(allVolumes))
	for _, v := range allVolumes {
		info, err := v.Info()
		if err == nil && info.Persistent {
			persistentVolumes = append(persistentVolumes, v)
		}
	}
	c.Assert(len(allVolumes), jc.GreaterThan, len(persistentVolumes))

	c.Assert(machine.Destroy(), jc.ErrorIsNil)

	// Cannot advance to Dead while there are detachable dynamic volumes.
	err = machine.EnsureDead()
	c.Assert(err, jc.Satisfies, state.IsHasAttachmentsError)
	c.Assert(err, gc.ErrorMatches, "machine 0 has attachments \\[volume-0\\]")
	s.obliterateVolumeAttachment(c, machine.MachineTag(), names.NewVolumeTag("0"))
	c.Assert(machine.EnsureDead(), jc.ErrorIsNil)
	c.Assert(machine.Remove(), jc.ErrorIsNil)

	// Machine is gone: non-detachable storage should be done too.
	allVolumes, err = im.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	// We should only have the persistent volume remaining.
	c.Assert(allVolumes, gc.HasLen, 1)
	c.Assert(allVolumes[0].Tag().String(), gc.Equals, "volume-0")

	attachments, err := im.MachineVolumeAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 0)
}

func (s *VolumeStateSuite) TestEnsureMachineDeadAddVolumeConcurrently(c *gc.C) {
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.MachineVolumeParams{{
			Volume: state.VolumeParams{Pool: "static", Size: 1024},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	addVolume := func() {
		_, u, _ := s.setupSingleStorage(c, "block", "modelscoped")
		err := u.AssignToMachine(machine)
		c.Assert(err, jc.ErrorIsNil)
		s.obliterateUnit(c, u.UnitTag())
	}
	defer state.SetBeforeHooks(c, s.State, addVolume).Check()

	// The static volume the machine was provisioned with does not matter,
	// but the volume added concurrently does.
	err = machine.EnsureDead()
	c.Assert(err, gc.ErrorMatches, `machine 0 has attachments \[volume-1\]`)
}

func (s *VolumeStateSuite) TestEnsureMachineDeadRemoveVolumeConcurrently(c *gc.C) {
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.MachineVolumeParams{{
			Volume: state.VolumeParams{Pool: "static", Size: 1024},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	removeVolume := func() {
		s.obliterateVolume(c, names.NewVolumeTag("0"))
	}
	defer state.SetBeforeHooks(c, s.State, removeVolume).Check()

	// Removing a volume concurrently does not cause a transaction failure.
	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *VolumeStateSuite) TestVolumeMachineScoped(c *gc.C) {
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.MachineVolumeParams{{
			Volume: state.VolumeParams{Pool: "loop", Size: 1024},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	volume := s.volume(c, names.NewVolumeTag("0/0"))
	c.Assert(volume.Life(), gc.Equals, state.Alive)

	err = im.DestroyVolume(volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	volume = s.volume(c, volume.VolumeTag())
	c.Assert(volume.Life(), gc.Equals, state.Dead)

	// Remove the machine: this should remove the volume.
	err = machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	volume, err = im.Volume(volume.VolumeTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *VolumeStateSuite) TestVolumeBindingStorage(c *gc.C) {
	// Volumes created assigned to a storage instance are bound
	// to the machine/model, and not the storage. i.e. storage
	// is persistent by default.
	volume, _, u := s.setupStorageVolumeAttachment(c)
	storageTag, err := volume.StorageInstance()
	c.Assert(err, jc.ErrorIsNil)

	// The volume should transition to Dying when the storage is removed.
	// We must destroy the unit before we can remove the storage.
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, s.State, storageTag)
	volume = s.volume(c, volume.VolumeTag())
	c.Assert(volume.Life(), gc.Equals, state.Dying)
}

func (s *VolumeStateSuite) setupStorageVolumeAttachment(c *gc.C) (state.Volume, *state.Machine, *state.Unit) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "modelscoped")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	machine := unitMachine(c, s.State, u)
	return s.storageInstanceVolume(c, storageTag), machine, u
}

func (s *VolumeStateSuite) setupModelScopedVolumeAttachment(c *gc.C) (state.Volume, *state.Machine) {
	return s.setupVolumeAttachment(c, "modelscoped")
}

func (s *VolumeStateSuite) setupMachineScopedVolumeAttachment(c *gc.C) (state.Volume, *state.Machine) {
	return s.setupVolumeAttachment(c, "loop")
}

func (s *VolumeStateSuite) setupVolumeAttachment(c *gc.C, pool string) (state.Volume, *state.Machine) {
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.MachineVolumeParams{{
			Volume: state.VolumeParams{Pool: pool, Size: 1024},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	volumeAttachments, err := im.MachineVolumeAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)
	volume, err := im.Volume(volumeAttachments[0].Volume())
	c.Assert(err, jc.ErrorIsNil)
	return volume, machine
}

func removeVolumeStorageInstance(c *gc.C, st *state.State, volumeTag names.VolumeTag) {
	im, err := st.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	volume, err := im.Volume(volumeTag)
	c.Assert(err, jc.ErrorIsNil)
	storageTag, err := volume.StorageInstance()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, st, storageTag)
}

func removeStorageInstance(c *gc.C, st *state.State, storageTag names.StorageTag) {
	err := st.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	attachments, err := st.StorageAttachments(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		err = st.DetachStorage(storageTag, a.Unit())
		c.Assert(err, jc.ErrorIsNil)
	}
	_, err = st.StorageInstance(storageTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
