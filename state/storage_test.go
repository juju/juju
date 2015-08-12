// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/storage/provider/registry"
)

type StorageStateSuite struct {
	StorageStateSuiteBase
}

var _ = gc.Suite(&StorageStateSuite{})

type StorageStateSuiteBase struct {
	ConnSuite
}

func (s *StorageStateSuiteBase) SetUpSuite(c *gc.C) {
	s.ConnSuite.SetUpSuite(c)

	registry.RegisterProvider("environscoped", &dummy.StorageProvider{
		StorageScope: storage.ScopeEnviron,
		IsDynamic:    true,
	})
	registry.RegisterProvider("machinescoped", &dummy.StorageProvider{
		StorageScope: storage.ScopeMachine,
		IsDynamic:    true,
	})
	registry.RegisterProvider("environscoped-block", &dummy.StorageProvider{
		StorageScope: storage.ScopeEnviron,
		SupportsFunc: func(k storage.StorageKind) bool {
			return k == storage.StorageKindBlock
		},
		IsDynamic: true,
	})
	registry.RegisterProvider("static", &dummy.StorageProvider{
		IsDynamic: false,
	})
	registry.RegisterEnvironStorageProviders(
		"someprovider", "environscoped", "machinescoped",
		"environscoped-block", "static",
	)
	s.AddSuiteCleanup(func(c *gc.C) {
		registry.RegisterProvider("environscoped", nil)
		registry.RegisterProvider("machinescoped", nil)
		registry.RegisterProvider("environscoped-block", nil)
		registry.RegisterProvider("static", nil)
	})
}

func (s *StorageStateSuiteBase) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	// Create a default pool for block devices.
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType)

	// Create a pool that creates persistent block devices.
	_, err = pm.Create("persistent-block", "environscoped-block", map[string]interface{}{
		"persistent": true,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) setupSingleStorage(c *gc.C, kind, pool string) (*state.Service, *state.Unit, names.StorageTag) {
	// There are test charms called "storage-block" and
	// "storage-filesystem" which are what you'd expect.
	ch := s.AddTestingCharm(c, "storage-"+kind)
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-"+kind, ch, storage)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	storageTag := names.NewStorageTag("data/0")
	return service, unit, storageTag
}

func (s *StorageStateSuiteBase) createStorageCharm(c *gc.C, charmName string, storageMeta charm.Storage) *state.Charm {
	return s.createStorageCharmRev(c, charmName, storageMeta, 1)
}

func (s *StorageStateSuiteBase) createStorageCharmRev(c *gc.C, charmName string, storageMeta charm.Storage, rev int) *state.Charm {
	meta := fmt.Sprintf(`
name: %s
summary: A charm for testing storage
description: ditto
storage:
  %s:
    type: %s
`, charmName, storageMeta.Name, storageMeta.Type)
	if storageMeta.ReadOnly {
		meta += "    read-only: true\n"
	}
	if storageMeta.Shared {
		meta += "    shared: true\n"
	}
	if storageMeta.MinimumSize > 0 {
		meta += fmt.Sprintf("    minimum-size: %dM\n", storageMeta.MinimumSize)
	}
	if storageMeta.Location != "" {
		meta += "    location: " + storageMeta.Location + "\n"
	}
	if storageMeta.CountMin != 1 || storageMeta.CountMax != 1 {
		meta += "    multiple:\n"
		meta += fmt.Sprintf("      range: %d-", storageMeta.CountMin)
		if storageMeta.CountMax >= 0 {
			meta += fmt.Sprint(storageMeta.CountMax)
		}
		meta += "\n"
	}
	ch := s.AddMetaCharm(c, charmName, meta, rev)
	return ch
}

func (s *StorageStateSuiteBase) setupMixedScopeStorageService(c *gc.C, kind string) *state.Service {
	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("environscoped", 1024, 1),
		"multi2up":   makeStorageCons("machinescoped", 2048, 2),
	}
	ch := s.AddTestingCharm(c, "storage-"+kind+"2")
	return s.AddTestingServiceWithStorage(c, "storage-"+kind+"2", ch, storageCons)
}

func (s *StorageStateSuite) storageInstanceExists(c *gc.C, tag names.StorageTag) bool {
	_, err := state.TxnRevno(
		s.State,
		state.StorageInstancesC,
		state.DocID(s.State, tag.Id()),
	)
	if err != nil {
		c.Assert(err, gc.Equals, mgo.ErrNotFound)
		return false
	}
	return true
}

func (s *StorageStateSuiteBase) assertFilesystemUnprovisioned(c *gc.C, tag names.FilesystemTag) {
	filesystem := s.filesystem(c, tag)
	_, err := filesystem.Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok := filesystem.Params()
	c.Assert(ok, jc.IsTrue)
}

func (s *StorageStateSuiteBase) assertFilesystemInfo(c *gc.C, tag names.FilesystemTag, expect state.FilesystemInfo) {
	filesystem := s.filesystem(c, tag)
	info, err := filesystem.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expect)
	_, ok := filesystem.Params()
	c.Assert(ok, jc.IsFalse)
}

func (s *StorageStateSuiteBase) assertFilesystemAttachmentUnprovisioned(c *gc.C, m names.MachineTag, f names.FilesystemTag) {
	filesystemAttachment := s.filesystemAttachment(c, m, f)
	_, err := filesystemAttachment.Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok := filesystemAttachment.Params()
	c.Assert(ok, jc.IsTrue)
}

func (s *StorageStateSuiteBase) assertFilesystemAttachmentInfo(c *gc.C, m names.MachineTag, f names.FilesystemTag, expect state.FilesystemAttachmentInfo) {
	filesystemAttachment := s.filesystemAttachment(c, m, f)
	info, err := filesystemAttachment.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expect)
	_, ok := filesystemAttachment.Params()
	c.Assert(ok, jc.IsFalse)
}

func (s *StorageStateSuiteBase) assertVolumeUnprovisioned(c *gc.C, tag names.VolumeTag) {
	volume := s.volume(c, tag)
	_, err := volume.Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok := volume.Params()
	c.Assert(ok, jc.IsTrue)
}

func (s *StorageStateSuiteBase) assertVolumeInfo(c *gc.C, tag names.VolumeTag, expect state.VolumeInfo) {
	volume := s.volume(c, tag)
	info, err := volume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expect)
	_, ok := volume.Params()
	c.Assert(ok, jc.IsFalse)
}

func (s *StorageStateSuiteBase) machine(c *gc.C, id string) *state.Machine {
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

func (s *StorageStateSuiteBase) filesystem(c *gc.C, tag names.FilesystemTag) state.Filesystem {
	filesystem, err := s.State.Filesystem(tag)
	c.Assert(err, jc.ErrorIsNil)
	return filesystem
}

func (s *StorageStateSuiteBase) filesystemVolume(c *gc.C, tag names.FilesystemTag) state.Volume {
	filesystem := s.filesystem(c, tag)
	volumeTag, err := filesystem.Volume()
	c.Assert(err, jc.ErrorIsNil)
	return s.volume(c, volumeTag)
}

func (s *StorageStateSuiteBase) filesystemAttachment(c *gc.C, m names.MachineTag, f names.FilesystemTag) state.FilesystemAttachment {
	attachment, err := s.State.FilesystemAttachment(m, f)
	c.Assert(err, jc.ErrorIsNil)
	return attachment
}

func (s *StorageStateSuiteBase) volume(c *gc.C, tag names.VolumeTag) state.Volume {
	volume, err := s.State.Volume(tag)
	c.Assert(err, jc.ErrorIsNil)
	return volume
}

func (s *StorageStateSuiteBase) volumeFilesystem(c *gc.C, tag names.VolumeTag) state.Filesystem {
	filesystem, err := s.State.VolumeFilesystem(tag)
	c.Assert(err, jc.ErrorIsNil)
	return filesystem
}

func (s *StorageStateSuiteBase) volumeAttachment(c *gc.C, m names.MachineTag, v names.VolumeTag) state.VolumeAttachment {
	attachment, err := s.State.VolumeAttachment(m, v)
	c.Assert(err, jc.ErrorIsNil)
	return attachment
}

func (s *StorageStateSuiteBase) storageInstanceVolume(c *gc.C, tag names.StorageTag) state.Volume {
	volume, err := s.State.StorageInstanceVolume(tag)
	c.Assert(err, jc.ErrorIsNil)
	return volume
}

func (s *StorageStateSuiteBase) storageInstanceFilesystem(c *gc.C, tag names.StorageTag) state.Filesystem {
	filesystem, err := s.State.StorageInstanceFilesystem(tag)
	c.Assert(err, jc.ErrorIsNil)
	return filesystem
}

func (s *StorageStateSuiteBase) obliterateUnit(c *gc.C, tag names.UnitTag) {
	u, err := s.State.Unit(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.obliterateUnitStorage(c, tag)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) obliterateUnitStorage(c *gc.C, tag names.UnitTag) {
	attachments, err := s.State.UnitStorageAttachments(tag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		err = s.State.DestroyStorageAttachment(a.StorageInstance(), a.Unit())
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.RemoveStorageAttachment(a.StorageInstance(), a.Unit())
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *StorageStateSuiteBase) obliterateVolume(c *gc.C, tag names.VolumeTag) {
	err := s.State.DestroyVolume(tag)
	if errors.IsNotFound(err) {
		return
	}
	attachments, err := s.State.VolumeAttachments(tag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		s.obliterateVolumeAttachment(c, a.Machine(), a.Volume())
	}
	err = s.State.RemoveVolume(tag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) obliterateVolumeAttachment(c *gc.C, m names.MachineTag, v names.VolumeTag) {
	err := s.State.DetachVolume(m, v)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveVolumeAttachment(m, v)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) obliterateFilesystem(c *gc.C, tag names.FilesystemTag) {
	err := s.State.DestroyFilesystem(tag)
	if errors.IsNotFound(err) {
		return
	}
	attachments, err := s.State.FilesystemAttachments(tag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		s.obliterateFilesystemAttachment(c, a.Machine(), a.Filesystem())
	}
	err = s.State.RemoveFilesystem(tag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) obliterateFilesystemAttachment(c *gc.C, m names.MachineTag, f names.FilesystemTag) {
	err := s.State.DetachFilesystem(m, f)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveFilesystemAttachment(m, f)
	c.Assert(err, jc.ErrorIsNil)
}

// assertMachineStorageRefs ensures that the specified machine's set of volume
// and filesystem references corresponds exactly to the volume and filesystem
// attachments that relate to the machine.
func assertMachineStorageRefs(c *gc.C, st *state.State, m names.MachineTag) {
	machines, closer := state.GetRawCollection(st, state.MachinesC)
	defer closer()

	var doc struct {
		Volumes     []string `bson:"volumes,omitempty"`
		Filesystems []string `bson:"filesystems,omitempty"`
	}
	err := machines.FindId(state.DocID(st, m.Id())).One(&doc)
	c.Assert(err, jc.ErrorIsNil)

	have := make(set.Tags)
	for _, v := range doc.Volumes {
		have.Add(names.NewVolumeTag(v))
	}
	for _, f := range doc.Filesystems {
		have.Add(names.NewFilesystemTag(f))
	}

	expect := make(set.Tags)
	volumeAttachments, err := st.MachineVolumeAttachments(m)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range volumeAttachments {
		expect.Add(a.Volume())
	}
	filesystemAttachments, err := st.MachineFilesystemAttachments(m)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range filesystemAttachments {
		expect.Add(a.Filesystem())
	}

	c.Assert(have, jc.DeepEquals, expect)
}

func makeStorageCons(pool string, size, count uint64) state.StorageConstraints {
	return state.StorageConstraints{Pool: pool, Size: size, Count: count}
}

func (s *StorageStateSuite) TestAddServiceStorageConstraintsDefault(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storageBlock, err := s.State.AddService("storage-block", "user-test-admin@local", ch, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	constraints, err := storageBlock.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(constraints, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Pool:  "loop",
			Count: 1,
			Size:  1024,
		},
		"allecto": {
			Pool:  "loop",
			Count: 0,
			Size:  1024,
		},
	})

	ch = s.AddTestingCharm(c, "storage-filesystem")
	storageFilesystem, err := s.State.AddService("storage-filesystem", "user-test-admin@local", ch, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	constraints, err = storageFilesystem.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(constraints, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Pool:  "rootfs",
			Count: 1,
			Size:  1024,
		},
	})
}

func (s *StorageStateSuite) TestAddServiceStorageConstraintsValidation(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block2")
	addService := func(storage map[string]state.StorageConstraints) (*state.Service, error) {
		return s.State.AddService("storage-block2", "user-test-admin@local", ch, nil, storage)
	}
	assertErr := func(storage map[string]state.StorageConstraints, expect string) {
		_, err := addService(storage)
		c.Assert(err, gc.ErrorMatches, expect)
	}

	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop-pool", 1024, 1),
		"multi2up":   makeStorageCons("loop-pool", 2048, 1),
	}
	assertErr(storageCons, `cannot add service "storage-block2": charm "storage-block2" store "multi2up": 2 instances required, 1 specified`)
	storageCons["multi2up"] = makeStorageCons("loop-pool", 1024, 2)
	assertErr(storageCons, `cannot add service "storage-block2": charm "storage-block2" store "multi2up": minimum storage size is 2.0GB, 1.0GB specified`)
	storageCons["multi2up"] = makeStorageCons("loop-pool", 2048, 2)
	storageCons["multi1to10"] = makeStorageCons("loop-pool", 1024, 11)
	assertErr(storageCons, `cannot add service "storage-block2": charm "storage-block2" store "multi1to10": at most 10 instances supported, 11 specified`)
	storageCons["multi1to10"] = makeStorageCons("ebs-fast", 1024, 10)
	assertErr(storageCons, `cannot add service "storage-block2": pool "ebs-fast" not found`)
	storageCons["multi1to10"] = makeStorageCons("loop-pool", 1024, 10)
	_, err := addService(storageCons)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuite) assertAddServiceStorageConstraintsDefaults(c *gc.C, pool string, cons, expect map[string]state.StorageConstraints) {
	if pool != "" {
		err := s.State.UpdateEnvironConfig(map[string]interface{}{
			"storage-default-block-source": pool,
		}, nil, nil)
		c.Assert(err, jc.ErrorIsNil)
	}
	ch := s.AddTestingCharm(c, "storage-block")
	service, err := s.State.AddService("storage-block2", "user-test-admin@local", ch, nil, cons)
	c.Assert(err, jc.ErrorIsNil)
	savedCons, err := service.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCons, jc.DeepEquals, expect)
	// TODO(wallyworld) - test pool name stored in data model
}

func (s *StorageStateSuite) TestAddServiceStorageConstraintsNoConstraintsUsed(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 0, 0),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data":    makeStorageCons("loop", 1024, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddServiceStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddServiceStorageConstraintsJustCount(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 0, 1),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data":    makeStorageCons("loop-pool", 1024, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddServiceStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddServiceStorageConstraintsDefaultPool(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 2048, 1),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data":    makeStorageCons("loop-pool", 2048, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddServiceStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddServiceStorageConstraintsNoUserDefaultPool(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 2048, 1),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data":    makeStorageCons("loop", 2048, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddServiceStorageConstraintsDefaults(c, "", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddServiceStorageConstraintsDefaultSizeFallback(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop-pool", 0, 1),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data":    makeStorageCons("loop-pool", 1024, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddServiceStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddServiceStorageConstraintsDefaultSizeFromCharm(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop", 0, 3),
	}
	expectedCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop", 1024, 3),
		"multi2up":   makeStorageCons("loop", 2048, 2),
	}
	ch := s.AddTestingCharm(c, "storage-block2")
	service, err := s.State.AddService("storage-block2", "user-test-admin@local", ch, nil, storageCons)
	c.Assert(err, jc.ErrorIsNil)
	savedCons, err := service.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCons, jc.DeepEquals, expectedCons)
}

func (s *StorageStateSuite) TestProviderFallbackToType(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	addService := func(storage map[string]state.StorageConstraints) (*state.Service, error) {
		return s.State.AddService("storage-block", "user-test-admin@local", ch, nil, storage)
	}
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop", 1024, 1),
	}
	_, err := addService(storageCons)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuite) TestAddUnit(c *gc.C) {
	s.assertStorageUnitsAdded(c)
}

func (s *StorageStateSuite) assertStorageUnitsAdded(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{
		"storage-default-block-source": "loop-pool",
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Each unit added to the service will create storage instances
	// to satisfy the service's storage constraints.
	ch := s.AddTestingCharm(c, "storage-block2")
	storage := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("", 1024, 1),
		"multi2up":   makeStorageCons("loop-pool", 2048, 2),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block2", ch, storage)
	for i := 0; i < 2; i++ {
		u, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		storageAttachments, err := s.State.UnitStorageAttachments(u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		count := make(map[string]int)
		for _, att := range storageAttachments {
			c.Assert(att.Unit(), gc.Equals, u.UnitTag())
			storageInstance, err := s.State.StorageInstance(att.StorageInstance())
			c.Assert(err, jc.ErrorIsNil)
			count[storageInstance.StorageName()]++
			c.Assert(storageInstance.Kind(), gc.Equals, state.StorageKindBlock)
			c.Assert(storageInstance.CharmURL(), gc.DeepEquals, ch.URL())
		}
		c.Assert(count, gc.DeepEquals, map[string]int{
			"multi1to10": 1,
			"multi2up":   2,
		})
		// TODO(wallyworld) - test pool name stored in data model
	}
}

func (s *StorageStateSuite) TestAllStorageInstances(c *gc.C) {
	s.assertStorageUnitsAdded(c)

	all, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 6)

	nameSet := set.NewStrings("multi1to10", "multi2up")
	ownerSet := set.NewStrings("unit-storage-block2-0", "unit-storage-block2-1")

	for _, one := range all {
		c.Assert(one.Kind(), gc.DeepEquals, state.StorageKindBlock)
		c.Assert(nameSet.Contains(one.StorageName()), jc.IsTrue)
		c.Assert(ownerSet.Contains(one.Owner().String()), jc.IsTrue)
	}
}

func (s *StorageStateSuite) TestStorageAttachments(c *gc.C) {
	s.assertStorageUnitsAdded(c)

	assertAttachments := func(tag names.StorageTag, expect ...names.UnitTag) {
		attachments, err := s.State.StorageAttachments(tag)
		c.Assert(err, jc.ErrorIsNil)
		units := make([]names.UnitTag, len(attachments))
		for i, a := range attachments {
			units[i] = a.Unit()
		}
		c.Assert(units, jc.SameContents, expect)
	}

	u0 := names.NewUnitTag("storage-block2/0")
	u1 := names.NewUnitTag("storage-block2/1")

	assertAttachments(names.NewStorageTag("multi1to10/0"), u0)
	assertAttachments(names.NewStorageTag("multi2up/1"), u0)
	assertAttachments(names.NewStorageTag("multi2up/2"), u0)
	assertAttachments(names.NewStorageTag("multi1to10/3"), u1)
	assertAttachments(names.NewStorageTag("multi2up/4"), u1)
	assertAttachments(names.NewStorageTag("multi2up/5"), u1)
}

func (s *StorageStateSuite) TestAllStorageInstancesEmpty(c *gc.C) {
	all, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}

func (s *StorageStateSuite) TestUnitEnsureDead(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	// destroying a unit with storage attachments is fine; this is what
	// will trigger the death and removal of storage attachments.
	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// until all storage attachments are removed, the unit cannot be
	// marked as being dead.
	assertUnitEnsureDeadError := func() {
		err = u.EnsureDead()
		c.Assert(err, gc.ErrorMatches, "unit has storage attachments")
	}
	assertUnitEnsureDeadError()
	err = s.State.DestroyStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	assertUnitEnsureDeadError()
	err = s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	assertUnitEnsureDeadError()
	err = s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuite) TestRemoveStorageAttachmentsRemovesDyingInstance(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	// Mark the storage instance as Dying, so that it will be removed
	// when the last attachment is removed.
	err := s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)

	si, err := s.State.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Life(), gc.Equals, state.Dying)

	err = s.State.DestroyStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestRemoveStorageAttachmentsRemovesUnitOwnedInstance(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	// Even though the storage instance is Alive, it will be removed when
	// the last attachment is removed, since it is not possible to add
	// more attachments later.
	si, err := s.State.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Life(), gc.Equals, state.Alive)

	err = s.State.DestroyStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestConcurrentDestroyStorageInstanceRemoveStorageAttachmentsRemovesInstance(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.DestroyStorageAttachment(storageTag, u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	// Destroying the instance should check that there are no concurrent
	// changes to the storage instance's attachments, and recompute
	// operations if there are.
	err := s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)

	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestConcurrentRemoveStorageAttachment(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	err := s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)

	destroy := func() {
		err = s.State.DestroyStorageAttachment(storageTag, u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
	}
	remove := func() {
		err = s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
	}

	defer state.SetBeforeHooks(c, s.State, destroy, remove).Check()
	destroy()
	remove()
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestRemoveAliveStorageAttachmentError(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	err := s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, gc.ErrorMatches, "cannot remove storage attachment data/0:storage-block/0: storage attachment is not dying")

	attachments, err := s.State.UnitStorageAttachments(u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 1)
	c.Assert(attachments[0].StorageInstance(), gc.Equals, storageTag)
}

func (s *StorageStateSuite) TestConcurrentDestroyInstanceRemoveStorageAttachmentsRemovesInstance(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	defer state.SetBeforeHooks(c, s.State, func() {
		// Concurrently mark the storage instance as Dying,
		// so that it will be removed when the last attachment
		// is removed.
		err := s.State.DestroyStorageInstance(storageTag)
		c.Assert(err, jc.ErrorIsNil)
	}, nil).Check()

	// Removing the attachment should check that there are no concurrent
	// changes to the storage instance's life, and recompute operations
	// if it does.
	err := s.State.DestroyStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestConcurrentDestroyStorageInstance(c *gc.C) {
	_, _, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.DestroyStorageInstance(storageTag)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)

	si, err := s.State.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Life(), gc.Equals, state.Dying)
}

func (s *StorageStateSuite) TestWatchStorageAttachments(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block2")
	storage := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop-pool", 1024, 1),
		"multi2up":   makeStorageCons("loop-pool", 2048, 2),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block2", ch, storage)
	u, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	w := s.State.WatchStorageAttachments(u.UnitTag())
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange("multi1to10/0", "multi2up/1", "multi2up/2")
	wc.AssertNoChange()

	err = s.State.DestroyStorageAttachment(names.NewStorageTag("multi2up/1"), u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("multi2up/1")
	wc.AssertNoChange()
}

func (s *StorageStateSuite) TestWatchStorageAttachment(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	w := s.State.WatchStorageAttachment(storageTag, u.UnitTag())
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	err := s.State.DestroyStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	err = s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *StorageStateSuite) TestDestroyUnitStorageAttachments(c *gc.C) {
	service := s.setupMixedScopeStorageService(c, "block")
	u, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.DestroyUnitStorageAttachments(u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		attachments, err := s.State.UnitStorageAttachments(u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(attachments, gc.HasLen, 3)
		for _, a := range attachments {
			c.Assert(a.Life(), gc.Equals, state.Dying)
			err := s.State.RemoveStorageAttachment(a.StorageInstance(), u.UnitTag())
			c.Assert(err, jc.ErrorIsNil)
		}
	}).Check()

	err = s.State.DestroyUnitStorageAttachments(u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuite) TestStorageLocationConflictIdentical(c *gc.C) {
	s.testStorageLocationConflict(
		c, "/srv", "/srv",
		`cannot assign unit "storage-filesystem2/0" to machine 0: `+
			`validating filesystem mount points: `+
			`mount point "/srv" for "data-old" storage contains `+
			`mount point "/srv" for "data-new" storage`,
	)
}

func (s *StorageStateSuite) TestStorageLocationConflictIdenticalAfterCleaning(c *gc.C) {
	s.testStorageLocationConflict(
		c, "/srv", "/xyz/.././srv",
		`cannot assign unit "storage-filesystem2/0" to machine 0: `+
			`validating filesystem mount points: `+
			`mount point "/srv" for "data-old" storage contains `+
			`mount point "/xyz/.././srv" for "data-new" storage`,
	)
}

func (s *StorageStateSuite) TestStorageLocationConflictSecondInsideFirst(c *gc.C) {
	s.testStorageLocationConflict(
		c, "/srv", "/srv/within",
		`cannot assign unit "storage-filesystem2/0" to machine 0: `+
			`validating filesystem mount points: `+
			`mount point "/srv" for "data-old" storage contains `+
			`mount point "/srv/within" for "data-new" storage`,
	)
}

func (s *StorageStateSuite) TestStorageLocationConflictFirstInsideSecond(c *gc.C) {
	s.testStorageLocationConflict(
		c, "/srv/within", "/srv",
		`cannot assign unit "storage-filesystem2/0" to machine 0: `+
			`validating filesystem mount points: `+
			`mount point "/srv" for "data-new" storage contains `+
			`mount point "/srv/within" for "data-old" storage`,
	)
}

func (s *StorageStateSuite) TestStorageLocationConflictPrefix(c *gc.C) {
	s.testStorageLocationConflict(c, "/srv", "/srvtd", "")
}

func (s *StorageStateSuite) TestStorageLocationConflictSameParent(c *gc.C) {
	s.testStorageLocationConflict(c, "/srv/1", "/srv/2", "")
}

func (s *StorageStateSuite) TestStorageLocationConflictAutoGenerated(c *gc.C) {
	s.testStorageLocationConflict(c, "", "", "")
}

func (s *StorageStateSuite) testStorageLocationConflict(c *gc.C, first, second, expectErr string) {
	ch1 := s.createStorageCharm(c, "storage-filesystem", charm.Storage{
		Name:     "data-old",
		Type:     charm.StorageFilesystem,
		CountMin: 1,
		CountMax: 1,
		Location: first,
	})
	ch2 := s.createStorageCharm(c, "storage-filesystem2", charm.Storage{
		Name:     "data-new",
		Type:     charm.StorageFilesystem,
		CountMin: 1,
		CountMax: 1,
		Location: second,
	})
	svc1 := s.AddTestingService(c, "storage-filesystem", ch1)
	svc2 := s.AddTestingService(c, "storage-filesystem2", ch2)

	u1, err := svc1.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(u1, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	machineId, err := u1.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	u2, err := svc2.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = u2.AssignToMachine(m)
	if expectErr == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, expectErr)
	}
}

// TODO(axw) the following require shared storage support to test:
// - StorageAttachments can't be added to Dying StorageInstance
// - StorageInstance without attachments is removed by Destroy
// - concurrent add-unit and StorageAttachment removal does not
//   remove storage instance.
