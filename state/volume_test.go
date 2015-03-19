// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
)

type VolumeStateSuite struct {
	StorageStateSuiteBase
}

var _ = gc.Suite(&VolumeStateSuite{})

func (s *VolumeStateSuite) TestAddMachine(c *gc.C) {
	_, unit, _ := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	storageAttachments, err := s.State.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)
	storageInstance, err := s.State.StorageInstance(storageAttachments[0].StorageInstance())
	c.Assert(err, jc.ErrorIsNil)

	volume, err := s.State.StorageInstanceVolume(storageInstance.StorageTag())
	c.Assert(err, jc.ErrorIsNil)
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
	volumeAttachments, err := s.State.MachineVolumeAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)
	c.Assert(volumeAttachments[0].Volume(), gc.Equals, volume.VolumeTag())
	c.Assert(volumeAttachments[0].Machine(), gc.Equals, machine.MachineTag())
	_, err = volumeAttachments[0].Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok = volumeAttachments[0].Params()
	c.Assert(ok, jc.IsTrue)

	_, err = s.State.VolumeAttachment(machine.MachineTag(), volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *VolumeStateSuite) TestAddServiceInvalidPool(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("invalid-pool", 1024, 1),
	}
	_, err := s.State.AddService("storage-block", s.Owner.String(), ch, nil, storage)
	c.Assert(err, gc.ErrorMatches, `.* pool "invalid-pool" not found`)
}

func (s *VolumeStateSuite) TestAddServiceNoUserDefaultPool(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}
	service, err := s.State.AddService("storage-block", s.Owner.String(), ch, nil, storage)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := service.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": state.StorageConstraints{
			Pool:  "loop",
			Size:  1024,
			Count: 1,
		},
	})
}

func (s *VolumeStateSuite) TestAddServiceDefaultPool(c *gc.C) {
	// Register a default pool.
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("default-block", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.UpdateEnvironConfig(map[string]interface{}{
		"storage-default-block-source": "default-block",
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block", ch, storage)
	cons, err := service.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": state.StorageConstraints{
			Pool:  "default-block",
			Size:  1024,
			Count: 1,
		},
	})
}

func (s *VolumeStateSuite) TestSetVolumeInfo(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	volume, err := s.State.StorageInstanceVolume(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	volumeTag := volume.VolumeTag()
	s.assertVolumeUnprovisioned(c, volumeTag)

	volumeInfoSet := state.VolumeInfo{Size: 123, Persistent: true}
	err = s.State.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)
	volumeInfoSet.Pool = "loop-pool" // taken from params
	s.assertVolumeInfo(c, volumeTag, volumeInfoSet)
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

	volumeAttachments, err := s.State.MachineVolumeAttachments(m.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)
	volumeTag := volumeAttachments[0].Volume()

	volume, err := s.State.Volume(volumeTag)
	c.Assert(err, jc.ErrorIsNil)
	_, err = volume.StorageInstance()
	c.Assert(err, jc.Satisfies, errors.IsNotAssigned)

	s.assertVolumeUnprovisioned(c, volumeTag)
	volumeInfoSet := state.VolumeInfo{Size: 123}
	err = s.State.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)
	volumeInfoSet.Pool = "loop-pool" // taken from params
	s.assertVolumeInfo(c, volumeTag, volumeInfoSet)
}

func (s *VolumeStateSuite) TestSetVolumeInfoImmutable(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	volume, err := s.State.StorageInstanceVolume(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	volumeTag := volume.VolumeTag()

	volumeInfoSet := state.VolumeInfo{Size: 123}
	err = s.State.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	// The first call to SetVolumeInfo takes the pool name from
	// the params; the second does not, but it must not change
	// either. Callers are expected to get the existing info and
	// update it, leaving immutable values intact.
	err = s.State.SetVolumeInfo(volume.VolumeTag(), volumeInfoSet)
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

	volume, err := s.State.StorageInstanceVolume(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	volumeTag := volume.VolumeTag()

	w := s.State.WatchVolumeAttachment(machineTag, volumeTag)
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	err = s.State.SetVolumeAttachmentInfo(
		machineTag, volumeTag, state.VolumeAttachmentInfo{
			DeviceName: "xvdf1",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// volume attachment will NOT react to volume changes
	err = s.State.SetVolumeInfo(volumeTag, state.VolumeInfo{VolumeId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *VolumeStateSuite) TestWatchEnvironVolumes(c *gc.C) {
	service := s.setupMixedScopeStorageService(c, "block")
	addUnit := func() {
		u, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	addUnit()

	w := s.State.WatchEnvironVolumes()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChangeInSingleEvent("3")
	wc.AssertNoChange()

	// TODO(axw) respond to Dying/Dead when we have
	// the means to progress Volume lifecycle.
}

func (s *VolumeStateSuite) TestWatchEnvironVolumeAttachments(c *gc.C) {
	service := s.setupMixedScopeStorageService(c, "block")
	addUnit := func() {
		u, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	addUnit()

	w := s.State.WatchEnvironVolumeAttachments()
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

func (s *VolumeStateSuite) TestWatchMachineVolumes(c *gc.C) {
	service := s.setupMixedScopeStorageService(c, "block")
	addUnit := func() {
		u, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	addUnit()

	w := s.State.WatchMachineVolumes(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0/1", "0/2") // initial
	wc.AssertNoChange()

	addUnit()
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	// TODO(axw) respond to Dying/Dead when we have
	// the means to progress Volume lifecycle.
}

func (s *VolumeStateSuite) TestWatchMachineVolumeAttachments(c *gc.C) {
	service := s.setupMixedScopeStorageService(c, "block")
	addUnit := func() {
		u, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	addUnit()

	w := s.State.WatchMachineVolumeAttachments(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("0:0", "0:0/1", "0:0/2") // initial
	wc.AssertNoChange()

	addUnit()
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	// TODO(axw) respond to changes to the same machine when we support
	// dynamic storage and/or placement.
	// TODO(axw) respond to Dying/Dead when we have
	// the means to progress Volume lifecycle.
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
	assertValid("0/lxc/0:1", names.NewMachineTag("0/lxc/0"), names.NewVolumeTag("1"))
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

func (s *VolumeStateSuite) assertVolumeUnprovisioned(c *gc.C, tag names.VolumeTag) {
	volume, err := s.State.Volume(tag)
	c.Assert(err, jc.ErrorIsNil)
	_, err = volume.Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok := volume.Params()
	c.Assert(ok, jc.IsTrue)
}

func (s *VolumeStateSuite) assertVolumeInfo(c *gc.C, tag names.VolumeTag, expect state.VolumeInfo) {
	volume, err := s.State.Volume(tag)
	c.Assert(err, jc.ErrorIsNil)
	info, err := volume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expect)
	_, ok := volume.Params()
	c.Assert(ok, jc.IsFalse)
}

func (s *VolumeStateSuite) TestPersistentVolumes(c *gc.C) {
	registry.RegisterEnvironStorageProviders("someprovider", ec2.EBS_ProviderType)
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("persistent-block", ec2.EBS_ProviderType, map[string]interface{}{"persistent": "true"})
	c.Assert(err, jc.ErrorIsNil)

	ch := s.AddTestingCharm(c, "storage-block2")
	storage := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("persistent-block", 1024, 1),
		"multi2up":   makeStorageCons("loop-pool", 2048, 2),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block2", ch, storage)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	volume1, err := s.State.StorageInstanceVolume(names.NewStorageTag("multi1to10/0"))
	c.Assert(err, jc.ErrorIsNil)

	volumeInfoSet := state.VolumeInfo{Size: 123, Persistent: true}
	err = s.State.SetVolumeInfo(volume1.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	volume2, err := s.State.StorageInstanceVolume(names.NewStorageTag("multi2up/1"))
	c.Assert(err, jc.ErrorIsNil)

	volumeInfoSet = state.VolumeInfo{Size: 456, Persistent: false}
	err = s.State.SetVolumeInfo(volume2.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	v, err := s.State.PersistentVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.HasLen, 1)
	c.Assert(v[0].VolumeTag(), gc.DeepEquals, volume1.VolumeTag())
}
