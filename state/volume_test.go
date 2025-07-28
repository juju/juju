// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
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
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineVolume(c, unit)
}

func (s *VolumeStateSuite) assertMachineVolume(c *gc.C, unit *state.Unit) {
	assignedMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	storageAttachments, err := s.storageBackend.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)
	storageInstance, err := s.storageBackend.StorageInstance(storageAttachments[0].StorageInstance())
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

	volumeAttachments, err := s.storageBackend.MachineVolumeAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)
	c.Assert(volumeAttachments[0].Volume(), gc.Equals, volume.VolumeTag())
	c.Assert(volumeAttachments[0].Host(), gc.Equals, machine.MachineTag())
	_, err = volumeAttachments[0].Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok = volumeAttachments[0].Params()
	c.Assert(ok, jc.IsTrue)

	_, err = s.storageBackend.VolumeAttachment(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)

	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
}

func (s *VolumeStateSuite) TestAddApplicationInvalidPool(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	testStorage := map[string]state.StorageConstraints{
		"data": makeStorageCons("invalid-pool", 1024, 1),
	}
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "storage-block", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: testStorage,
	})
	c.Assert(err, gc.ErrorMatches, `.* pool "invalid-pool" not found`)
}

func (s *VolumeStateSuite) TestAddApplicationNoUserDefaultPool(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	testStorage := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "storage-block", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: testStorage,
	})
	c.Assert(err, jc.ErrorIsNil)
	cons, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Pool:  "loop",
			Size:  1024,
			Count: 1,
		},
		"allecto": {
			Pool:  "loop",
			Size:  1024,
			Count: 0,
		},
	})
}

func (s *VolumeStateSuite) TestAddApplicationDefaultPool(c *gc.C) {
	// Register a default pool.
	pm := poolmanager.New(state.NewStateSettings(s.State), storage.ChainedProviderRegistry{
		dummystorage.StorageProviders(),
		provider.CommonStorageProviders(),
	})
	_, err := pm.Create("default-block", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.Model.UpdateModelConfig(map[string]interface{}{
		"storage-default-block-source": "default-block",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	ch := s.AddTestingCharm(c, "storage-block")
	testStorage := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}
	app := s.AddTestingApplicationWithStorage(c, "storage-block", ch, testStorage)
	cons, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Pool:  "default-block",
			Size:  1024,
			Count: 1,
		},
		"allecto": {
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

	volumeInfoSet := state.VolumeInfo{Size: 123, Persistent: true, VolumeId: "vol-ume"}
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
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

	volumeInfoSet := state.VolumeInfo{Size: 123, Persistent: true}
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
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
		Base:                    state.UbuntuBase("12.10"),
		Constraints:             cons,
		HardwareCharacteristics: hc,
		InstanceId:              "inst-id",
		Nonce:                   "nonce",
		Jobs:                    oneJob,
		Volumes: []state.HostVolumeParams{{
			Volume: volumeParams,
		}},
	}
	machines, err := s.State.AddMachines(machineTemplate)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	m, err := s.State.Machine(machines[0].Id())
	c.Assert(err, jc.ErrorIsNil)

	volumeAttachments, err := s.storageBackend.MachineVolumeAttachments(m.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)
	volumeTag := volumeAttachments[0].Volume()

	volume := s.volume(c, volumeTag)
	_, err = volume.StorageInstance()
	c.Assert(err, jc.Satisfies, errors.IsNotAssigned)

	s.assertVolumeUnprovisioned(c, volumeTag)
	volumeInfoSet := state.VolumeInfo{Size: 123, VolumeId: "vol-ume"}
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
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

	volumeInfoSet := state.VolumeInfo{Size: 123, VolumeId: "vol-ume"}
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	// The first call to SetVolumeInfo takes the pool name from
	// the params; the second does not, but it must not change
	// either. Callers are expected to get the existing info and
	// update it, leaving immutable values intact.
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
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
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchVolumeAttachment(machineTag, volumeTag)
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	machine, err := s.State.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	// volume attachment will NOT react to volume changes
	err = s.storageBackend.SetVolumeInfo(volumeTag, state.VolumeInfo{VolumeId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.storageBackend.SetVolumeAttachmentInfo(
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

	w := s.storageBackend.WatchModelVolumes()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0", "1") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChange("4", "5")
	wc.AssertNoChange()

	volume, err := s.storageBackend.Volume(names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	storageTag, err := volume.StorageInstance()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, s.storageBackend, storageTag)
	err = s.storageBackend.DestroyVolume(names.NewVolumeTag("0"), false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0") // dying
	wc.AssertNoChange()

	err = s.storageBackend.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("0"), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0"), false)
	c.Assert(err, jc.ErrorIsNil)

	err = s.storageBackend.RemoveVolume(names.NewVolumeTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0") // removed
	wc.AssertNoChange()
}

func (s *VolumeStateSuite) TestWatchModelVolumeAttachments(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "block")
	addUnit := func() {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	addUnit()

	w := s.storageBackend.WatchModelVolumeAttachments()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0:0", "0:1") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChange("1:4", "1:5")
	wc.AssertNoChange()

	err := s.storageBackend.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("0"), false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0:0") // dying
	wc.AssertNoChange()

	err = s.storageBackend.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0"), false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0:0") // removed
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

	w := s.storageBackend.WatchMachineVolumes(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0/0", "0/1") // initial
	wc.AssertNoChange()

	addUnit()
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	volume, err := s.storageBackend.Volume(names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	storageTag, err := volume.StorageInstance()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, s.storageBackend, storageTag)
	err = s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0/0") // dying
	wc.AssertNoChange()

	err = s.storageBackend.DestroyVolume(names.NewVolumeTag("0/0"), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0/0"), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolume(names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0/0") // removed
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

	w := s.storageBackend.WatchMachineVolumeAttachments(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0:0/0", "0:0/1") // initial
	wc.AssertNoChange()

	addUnit(nil)
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	err := s.storageBackend.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("2"), false)
	c.Assert(err, jc.ErrorIsNil)
	// no change, since we're only interested in attachments of
	// machine-scoped volumes.
	wc.AssertNoChange()

	removeVolumeStorageInstance(c, s.storageBackend, names.NewVolumeTag("0/0"))
	err = s.storageBackend.DestroyVolume(names.NewVolumeTag("0/0"), false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0:0/0") // dying
	wc.AssertNoChange()

	err = s.storageBackend.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0/0"), false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("0:0/0") // removed
	wc.AssertNoChange()

	addUnit(m0)
	wc.AssertChange("0:0/8", "0:0/9") // added
}

func (s *VolumeStateSuite) TestParseVolumeAttachmentId(c *gc.C) {
	assertValid := func(id string, m names.Tag, v names.VolumeTag) {
		hostTag, volumeTag, err := state.ParseVolumeAttachmentId(id)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(hostTag, gc.Equals, m)
		c.Assert(volumeTag, gc.Equals, v)
	}
	assertValid("0:0", names.NewMachineTag("0"), names.NewVolumeTag("0"))
	assertValid("0:0/1", names.NewMachineTag("0"), names.NewVolumeTag("0/1"))
	assertValid("0/lxd/0:1", names.NewMachineTag("0/lxd/0"), names.NewVolumeTag("1"))
	assertValid("some-unit/0:1", names.NewUnitTag("some-unit/0"), names.NewVolumeTag("1"))
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

	volumes, err := s.storageBackend.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	tags := make([]names.VolumeTag, len(volumes))
	for i, v := range volumes {
		tags[i] = v.VolumeTag()
	}
	c.Assert(tags, jc.SameContents, expected)
}

func (s *VolumeStateSuite) assertCreateVolumes(c *gc.C) (_ *state.Machine, all, persistent []names.VolumeTag) {
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{Pool: "persistent-block", Size: 1024},
		}, {
			Volume: state.VolumeParams{Pool: "loop-pool", Size: 2048},
		}, {
			Volume: state.VolumeParams{Pool: "static", Size: 2048},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())

	volume1 := s.volume(c, names.NewVolumeTag("0"))
	volume2 := s.volume(c, names.NewVolumeTag("0/1"))
	volume3 := s.volume(c, names.NewVolumeTag("2"))

	volumeInfoSet := state.VolumeInfo{Size: 123, Persistent: true, VolumeId: "vol-1"}
	err = s.storageBackend.SetVolumeInfo(volume1.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	volumeInfoSet = state.VolumeInfo{Size: 456, Persistent: false, VolumeId: "vol-2"}
	err = s.storageBackend.SetVolumeInfo(volume2.VolumeTag(), volumeInfoSet)
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

	// Provision volume attachment so that detaching the storage
	// attachment does not short-circuit.
	defer state.SetBeforeHooks(c, s.State, func() {
		machine := unitMachine(c, s.State, u)
		err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{VolumeId: "vol-123"})
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.SetVolumeAttachmentInfo(
			machine.MachineTag(), volume.VolumeTag(),
			state.VolumeAttachmentInfo{DeviceName: "xvdf1"},
		)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance and attachment are dying, but not yet
	// removed from state. The volume should still be assigned.
	s.storageInstanceVolume(c, storageTag)

	err = s.storageBackend.RemoveStorageAttachment(storageTag, u.UnitTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance is now gone; the volume should no longer
	// be assigned to the storage.
	_, err = s.storageBackend.StorageInstanceVolume(storageTag)
	c.Assert(err, gc.ErrorMatches, `volume for storage instance "data/0" not found`)

	// The volume should still exist, but it should be dying.
	v := s.volume(c, volume.VolumeTag())
	c.Assert(v.Life(), gc.Equals, state.Dying)
}

func (s *VolumeStateSuite) TestReleaseStorageInstanceVolumeReleasing(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "modelscoped")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	volume := s.storageInstanceVolume(c, storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volume.Releasing(), jc.IsFalse)
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{VolumeId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)

	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.ReleaseStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	// The volume should should be dying, and releasing.
	volume = s.volume(c, volume.VolumeTag())
	c.Assert(volume.Life(), gc.Equals, state.Dying)
	c.Assert(volume.Releasing(), jc.IsTrue)
}

func (s *VolumeStateSuite) TestReleaseStorageInstanceVolumeUnreleasable(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "modelscoped-unreleasable")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	volume := s.storageInstanceVolume(c, storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volume.Releasing(), jc.IsFalse)
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{VolumeId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)

	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.ReleaseStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, gc.ErrorMatches,
		`cannot release storage "data/0": storage provider "modelscoped-unreleasable" does not support releasing storage`,
	)
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	// The volume should should still be alive.
	volume = s.volume(c, volume.VolumeTag())
	c.Assert(volume.Life(), gc.Equals, state.Alive)
	c.Assert(volume.Releasing(), jc.IsFalse)
}

func (s *VolumeStateSuite) TestSetVolumeAttachmentInfoVolumeNotProvisioned(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(assignedMachineId)

	volume := s.storageInstanceVolume(c, storageTag)
	volumeTag := volume.VolumeTag()

	err = s.storageBackend.SetVolumeAttachmentInfo(
		machineTag, volumeTag, state.VolumeAttachmentInfo{
			DeviceName: "xvdf1",
		},
	)
	c.Assert(err, gc.ErrorMatches, `cannot set info for volume attachment 0/0:0: volume "0/0" not provisioned`)
}

func (s *VolumeStateSuite) TestDestroyVolume(c *gc.C) {
	volume, _ := s.setupMachineScopedVolumeAttachment(c)
	assertDestroy := func() {
		err := s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
		c.Assert(err, jc.ErrorIsNil)
		volume = s.volume(c, volume.VolumeTag())
		c.Assert(volume.Life(), gc.Equals, state.Dying)
	}
	defer state.SetBeforeHooks(c, s.State, assertDestroy).Check()
	assertDestroy()
}

func (s *VolumeStateSuite) TestDestroyVolumeNotFound(c *gc.C) {
	err := s.storageBackend.DestroyVolume(names.NewVolumeTag("0"), false)
	c.Assert(err, gc.ErrorMatches, `destroying volume 0: volume "0" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *VolumeStateSuite) TestDestroyVolumeStorageAssigned(c *gc.C) {
	volume, _, u := s.setupStorageVolumeAttachment(c)
	storageTag, err := volume.StorageInstance()
	c.Assert(err, jc.ErrorIsNil)

	err = s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, gc.ErrorMatches, "destroying volume 0: volume is assigned to storage data/0")

	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, s.storageBackend, storageTag)
	err = s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *VolumeStateSuite) TestDestroyVolumeNoAttachments(c *gc.C) {
	volume, machine := s.setupModelScopedVolumeAttachment(c)
	err := s.storageBackend.DetachVolume(machine.MachineTag(), volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.storageBackend.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag(), false)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	volume = s.volume(c, volume.VolumeTag())

	// There are no more attachments, so the volume should
	// have been progressed directly to Dead.
	c.Assert(volume.Life(), gc.Equals, state.Dead)
}

func (s *VolumeStateSuite) TestDetachVolumeDyingAttachment(c *gc.C) {
	volume, machine := s.setupModelScopedVolumeAttachment(c)
	volumeTag := volume.VolumeTag()
	machineTag := machine.MachineTag()
	// Make sure the state is already dying by the time we call DetachVolume
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.storageBackend.DetachVolume(machineTag, volumeTag, false)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := s.storageBackend.DetachVolume(machineTag, volumeTag, false)
	c.Assert(err, jc.ErrorIsNil)

	volumeAttachment := s.volumeAttachment(c, machineTag, volumeTag)
	c.Assert(volumeAttachment.Life(), gc.Equals, state.Dying)
	volume = s.volume(c, volumeTag)
	c.Assert(volume.Life(), gc.Equals, state.Alive)
	err = s.storageBackend.DestroyVolume(volumeTag, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *VolumeStateSuite) TestDetachVolumeDyingAttachmentPlan(c *gc.C) {
	volume, machine := s.setupModelScopedVolumeAttachment(c)
	machineTag := machine.MachineTag()
	volumeTag := volume.VolumeTag()
	err := machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeInfo(volumeTag,
		state.VolumeInfo{
			Size:     1024,
			VolumeId: "vol-ume",
		})
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeAttachmentInfo(machineTag, volumeTag,
		state.VolumeAttachmentInfo{
			DeviceName: "bogus",
		})
	c.Assert(err, jc.ErrorIsNil)
	// Simulate a machine agent recording its intent to attach the volume
	err = s.storageBackend.CreateVolumeAttachmentPlan(machineTag, volumeTag,
		state.VolumeAttachmentPlanInfo{DeviceType: storage.DeviceTypeLocal})
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.storageBackend.DetachVolume(machineTag, volumeTag, false)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = s.storageBackend.DetachVolume(machineTag, volumeTag, false)
	c.Assert(err, jc.ErrorIsNil)
	// The volume attachment shouldn't progress to dying, but its volume attachment plan should have
	volumeAttachment := s.volumeAttachment(c, machineTag, volumeTag)
	c.Assert(volumeAttachment.Life(), gc.Equals, state.Alive)
	volumeAttachmentPlan := s.volumeAttachmentPlan(c, machineTag, volumeTag)
	c.Assert(volumeAttachmentPlan.Life(), gc.Equals, state.Dying)
	volume = s.volume(c, volumeTag)
	// now calling RemoveAttachmentPlan removes the plan and moves the VolumeAttachment to dying
	err = s.storageBackend.RemoveVolumeAttachmentPlan(machineTag, volumeTag, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.storageBackend.VolumeAttachmentPlan(machineTag, volumeTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	volumeAttachment = s.volumeAttachment(c, machineTag, volumeTag)
	c.Assert(volumeAttachment.Life(), gc.Equals, state.Dying)
}

func (s *VolumeStateSuite) TestRemoveVolume(c *gc.C) {
	volume, machine := s.setupMachineScopedVolumeAttachment(c)

	err := s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	assertRemove := func() {
		err = s.storageBackend.RemoveVolume(volume.VolumeTag())
		c.Assert(err, jc.ErrorIsNil)
		_, err = s.storageBackend.Volume(volume.VolumeTag())
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
	defer state.SetBeforeHooks(c, s.State, assertRemove).Check()
	assertRemove()
}

func (s *VolumeStateSuite) TestRemoveVolumeNotFound(c *gc.C) {
	err := s.storageBackend.RemoveVolume(names.NewVolumeTag("42"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *VolumeStateSuite) TestRemoveVolumeNotDead(c *gc.C) {
	volume, _ := s.setupMachineScopedVolumeAttachment(c)
	err := s.storageBackend.RemoveVolume(volume.VolumeTag())
	c.Assert(err, gc.ErrorMatches, "removing volume 0/0: volume is not dead")
	err = s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolume(volume.VolumeTag())
	c.Assert(err, gc.ErrorMatches, "removing volume 0/0: volume is not dead")
}

func (s *VolumeStateSuite) TestDetachVolume(c *gc.C) {
	volume, machine := s.setupModelScopedVolumeAttachment(c)
	assertDetach := func() {
		err := s.storageBackend.DetachVolume(machine.MachineTag(), volume.VolumeTag(), false)
		c.Assert(err, jc.ErrorIsNil)
		attachment := s.volumeAttachment(c, machine.MachineTag(), volume.VolumeTag())
		c.Assert(attachment.Life(), gc.Equals, state.Dying)
	}
	defer state.SetBeforeHooks(c, s.State, assertDetach).Check()
	assertDetach()
}

func (s *VolumeStateSuite) TestDetachVolumeForce(c *gc.C) {
	volume, machine := s.setupModelScopedVolumeAttachment(c)
	coll, closer := state.GetCollection(s.st, "volumes")
	defer closer()

	// Set the volume to Dying even though the attachment is still alive.
	err := coll.Writeable().UpdateId(
		state.DocID(s.st, volume.VolumeTag().Id()),
		bson.D{{"$set", bson.D{{"life", state.Dying}}}})
	c.Assert(err, jc.ErrorIsNil)
	volume, err = s.storageBackend.Volume(volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volume.Life(), gc.Equals, state.Dying)

	assertDetach := func() {
		err := s.storageBackend.DetachVolume(machine.MachineTag(), volume.VolumeTag(), true)
		c.Assert(err, jc.ErrorIsNil)
		attachment := s.volumeAttachment(c, machine.MachineTag(), volume.VolumeTag())
		c.Assert(attachment.Life(), gc.Equals, state.Dying)
	}
	defer state.SetBeforeHooks(c, s.State, assertDetach).Check()
	assertDetach()
}

func (s *VolumeStateSuite) TestRemoveLastVolumeAttachment(c *gc.C) {
	volume, machine := s.setupModelScopedVolumeAttachment(c)

	err := s.storageBackend.DetachVolume(machine.MachineTag(), volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	err = s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	volume = s.volume(c, volume.VolumeTag())
	c.Assert(volume.Life(), gc.Equals, state.Dying)

	err = s.storageBackend.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	// The volume was Dying when the last attachment was
	// removed, so the volume should now be Dead.
	volume = s.volume(c, volume.VolumeTag())
	c.Assert(volume.Life(), gc.Equals, state.Dead)
}

func (s *VolumeStateSuite) TestRemoveLastVolumeAttachmentConcurrently(c *gc.C) {
	volume, machine := s.setupModelScopedVolumeAttachment(c)

	err := s.storageBackend.DetachVolume(machine.MachineTag(), volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
		c.Assert(err, jc.ErrorIsNil)
		volume := s.volume(c, volume.VolumeTag())
		c.Assert(volume.Life(), gc.Equals, state.Dying)
	}).Check()

	err = s.storageBackend.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	// Last attachment was removed, and the volume was (concurrently)
	// destroyed, so the volume should be Dead.
	volume = s.volume(c, volume.VolumeTag())
	c.Assert(volume.Life(), gc.Equals, state.Dead)
}

func (s *VolumeStateSuite) TestRemoveVolumeAttachmentNotFound(c *gc.C) {
	err := s.storageBackend.RemoveVolumeAttachment(names.NewMachineTag("42"), names.NewVolumeTag("42"), false)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `removing attachment of volume 42 from machine 42: volume "42" on "machine 42" not found`)
}

func (s *VolumeStateSuite) TestRemoveVolumeAttachmentConcurrently(c *gc.C) {
	volume, machine := s.setupMachineScopedVolumeAttachment(c)

	err := s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	remove := func() {
		err := s.storageBackend.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag(), false)
		c.Assert(err, jc.ErrorIsNil)
		assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
	}
	defer state.SetBeforeHooks(c, s.State, remove).Check()
	remove()
}

func (s *VolumeStateSuite) TestRemoveVolumeAttachmentAlive(c *gc.C) {
	volume, machine := s.setupMachineScopedVolumeAttachment(c)

	err := s.storageBackend.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag(), false)
	c.Assert(err, gc.ErrorMatches, "removing attachment of volume 0/0 from machine 0: volume attachment is not dying")
}

func (s *VolumeStateSuite) TestRemoveMachineRemovesVolumes(c *gc.C) {
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
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

	volumeInfoSet := state.VolumeInfo{Size: 123, Persistent: true, VolumeId: "vol-1"}
	err = s.storageBackend.SetVolumeInfo(names.NewVolumeTag("0/1"), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)
	volumeInfoSet = state.VolumeInfo{Size: 456, Persistent: false, VolumeId: "vol-2"}
	err = s.storageBackend.SetVolumeInfo(names.NewVolumeTag("0/3"), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)
	volumeInfoSet = state.VolumeInfo{Size: 789, Persistent: false, VolumeId: "vol-3"}
	err = s.storageBackend.SetVolumeInfo(names.NewVolumeTag("4"), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	allVolumes, err := s.storageBackend.AllVolumes()
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
	c.Assert(errors.Is(err, stateerrors.HasAttachmentsError), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "machine 0 has attachments \\[volume-0\\]")
	s.obliterateVolumeAttachment(c, machine.MachineTag(), names.NewVolumeTag("0"))
	c.Assert(machine.EnsureDead(), jc.ErrorIsNil)
	c.Assert(machine.Remove(), jc.ErrorIsNil)

	// Machine is gone: non-detachable storage should be done too.
	allVolumes, err = s.storageBackend.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	// We should only have the persistent volume remaining.
	c.Assert(allVolumes, gc.HasLen, 1)
	c.Assert(allVolumes[0].Tag().String(), gc.Equals, "volume-0")

	attachments, err := s.storageBackend.MachineVolumeAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 0)
}

func (s *VolumeStateSuite) TestEnsureMachineDeadAddVolumeConcurrently(c *gc.C) {
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
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
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
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
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{Pool: "loop", Size: 1024},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	volume := s.volume(c, names.NewVolumeTag("0/0"))
	c.Assert(volume.Life(), gc.Equals, state.Alive)

	err = s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(machine.MachineTag(), volume.VolumeTag(), false)
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
	volume, err = s.storageBackend.Volume(volume.VolumeTag())
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
	removeStorageInstance(c, s.storageBackend, storageTag)
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
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{Pool: pool, Size: 1024},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	volumeAttachments, err := s.storageBackend.MachineVolumeAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)
	volume, err := s.storageBackend.Volume(volumeAttachments[0].Volume())
	c.Assert(err, jc.ErrorIsNil)
	return volume, machine
}

func removeVolumeStorageInstance(c *gc.C, sb *state.StorageBackend, volumeTag names.VolumeTag) {
	volume, err := sb.Volume(volumeTag)
	c.Assert(err, jc.ErrorIsNil)
	storageTag, err := volume.StorageInstance()
	c.Assert(err, jc.ErrorIsNil)
	removeStorageInstance(c, sb, storageTag)
}

func removeStorageInstance(c *gc.C, sb *state.StorageBackend, storageTag names.StorageTag) {
	err := sb.DestroyStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	attachments, err := sb.StorageAttachments(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		err = sb.DetachStorage(storageTag, a.Unit(), false, dontWait)
		c.Assert(err, jc.ErrorIsNil)
	}
	_, err = sb.StorageInstance(storageTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *VolumeStateSuite) TestGetVolumeByVolumeId(c *gc.C) {
	// Create a storage instance with a volume and set its VolumeId
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	volume := s.storageInstanceVolume(c, storageTag)
	volumeId := "test-volume-123"

	// Set the volume info with a specific VolumeId
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{
		Pool:     "loop-pool",
		Size:     1024,
		VolumeId: volumeId,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Test GetVolumeByVolumeId - should find the volume
	foundVolume, err := s.storageBackend.GetVolumeByVolumeId(volumeId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundVolume.VolumeTag(), gc.Equals, volume.VolumeTag())

	// Verify the volume info matches
	foundInfo, err := foundVolume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundInfo.VolumeId, gc.Equals, volumeId)
	c.Assert(foundInfo.Pool, gc.Equals, "loop-pool")
	c.Assert(foundInfo.Size, gc.Equals, uint64(1024))
}

func (s *VolumeStateSuite) TestGetVolumeByVolumeIdNotFound(c *gc.C) {
	// Test GetVolumeByVolumeId with non-existent VolumeId
	_, err := s.storageBackend.GetVolumeByVolumeId("non-existent-volume")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `volume with volumeid "non-existent-volume" not found`)
}

func (s *VolumeStateSuite) TestGetVolumeByVolumeIdUnprovisionedVolume(c *gc.C) {
	// Create a volume but don't set its info (unprovisioned)
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	volume := s.storageInstanceVolume(c, storageTag)

	// Verify the volume exists but has no info
	_, err = volume.Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)

	// Test GetVolumeByVolumeId - should not find unprovisioned volume
	_, err = s.storageBackend.GetVolumeByVolumeId("any-volume-id")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `volume with volumeid "any-volume-id" not found`)
}
