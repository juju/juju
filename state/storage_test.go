// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/testing/factory"
)

type StorageStateSuiteBase struct {
	ConnSuite
}

func (s *StorageStateSuiteBase) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	// Create a default pool for block devices.
	pm := poolmanager.New(state.NewStateSettings(s.State), storage.ChainedProviderRegistry{
		dummy.StorageProviders(),
		provider.CommonStorageProviders(),
	})
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)

	// Create a pool that creates persistent block devices.
	_, err = pm.Create("persistent-block", "modelscoped-block", map[string]interface{}{
		"persistent": true,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) setupSingleStorage(c *gc.C, kind, pool string) (*state.Application, *state.Unit, names.StorageTag) {
	// There are test charms called "storage-block" and
	// "storage-filesystem" which are what you'd expect.
	ch := s.AddTestingCharm(c, "storage-"+kind)
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	app := s.AddTestingServiceWithStorage(c, "storage-"+kind, ch, storage)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	storageTag := names.NewStorageTag("data/0")
	return app, unit, storageTag
}

func (s *StorageStateSuiteBase) provisionStorageVolume(c *gc.C, u *state.Unit, storageTag names.StorageTag) {
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	machine := unitMachine(c, s.State, u)
	volume := s.storageInstanceVolume(c, storageTag)
	err = machine.SetProvisioned("inst-id", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = im.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{VolumeId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)
	err = im.SetVolumeAttachmentInfo(
		machine.MachineTag(),
		volume.VolumeTag(),
		state.VolumeAttachmentInfo{DeviceName: "sdc"},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) setupSingleStorageDetachable(c *gc.C, kind, pool string) (*state.Application, *state.Unit, names.StorageTag) {
	ch := s.createStorageCharm(c, "storage-"+kind, charm.Storage{
		Name:     "data",
		Type:     charm.StorageType(kind),
		CountMin: 0,
		CountMax: 2,
	})
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	app := s.AddTestingServiceWithStorage(c, ch.URL().Name, ch, storage)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	storageTag := names.NewStorageTag("data/0")
	return app, unit, storageTag
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

func (s *StorageStateSuiteBase) setupMixedScopeStorageApplication(
	c *gc.C, kind string, pools ...string,
) *state.Application {
	pool0 := "modelscoped"
	pool1 := "machinescoped"
	switch n := len(pools); n {
	default:
		c.Fatalf("invalid number of pools: %d", n)
	case 2:
		pool1 = pools[1]
		fallthrough
	case 1:
		pool0 = pools[0]
	case 0:
	}
	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons(pool0, 1024, 2),
		"multi2up":   makeStorageCons(pool1, 2048, 2),
	}
	ch := s.AddTestingCharm(c, "storage-"+kind+"2")
	return s.AddTestingServiceWithStorage(c, "storage-"+kind+"2", ch, storageCons)
}

func (s *StorageStateSuiteBase) storageInstanceExists(c *gc.C, tag names.StorageTag) bool {
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
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	volume, err := im.Volume(tag)
	c.Assert(err, jc.ErrorIsNil)
	return volume
}

func (s *StorageStateSuiteBase) volumeFilesystem(c *gc.C, tag names.VolumeTag) state.Filesystem {
	filesystem, err := s.State.VolumeFilesystem(tag)
	c.Assert(err, jc.ErrorIsNil)
	return filesystem
}

func (s *StorageStateSuiteBase) volumeAttachment(c *gc.C, m names.MachineTag, v names.VolumeTag) state.VolumeAttachment {
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	attachment, err := im.VolumeAttachment(m, v)
	c.Assert(err, jc.ErrorIsNil)
	return attachment
}

func (s *StorageStateSuiteBase) storageInstanceVolume(c *gc.C, tag names.StorageTag) state.Volume {
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	volume, err := im.StorageInstanceVolume(tag)
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
		err = s.State.DetachStorage(a.StorageInstance(), a.Unit())
		c.Assert(err, jc.ErrorIsNil)
		if _, err := s.State.StorageAttachment(a.StorageInstance(), a.Unit()); err == nil {
			err = s.State.RemoveStorageAttachment(a.StorageInstance(), a.Unit())
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func (s *StorageStateSuiteBase) obliterateVolume(c *gc.C, tag names.VolumeTag) {
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = im.DestroyVolume(tag)
	if errors.IsNotFound(err) {
		return
	}
	attachments, err := im.VolumeAttachments(tag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		s.obliterateVolumeAttachment(c, a.Machine(), a.Volume())
	}
	err = im.RemoveVolume(tag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) obliterateVolumeAttachment(c *gc.C, m names.MachineTag, v names.VolumeTag) {
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = im.DetachVolume(m, v)
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolumeAttachment(m, v)
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

	im, err := st.IAASModel()
	c.Assert(err, jc.ErrorIsNil)

	expect := make(set.Tags)
	volumeAttachments, err := im.MachineVolumeAttachments(m)
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

type StorageStateSuite struct {
	StorageStateSuiteBase
}

var _ = gc.Suite(&StorageStateSuite{})

func (s *StorageStateSuite) TestAddServiceStorageConstraintsDefault(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storageBlock, err := s.State.AddApplication(state.AddApplicationArgs{Name: "storage-block", Charm: ch})
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
	storageFilesystem, err := s.State.AddApplication(state.AddApplicationArgs{Name: "storage-filesystem", Charm: ch})
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
	addService := func(storage map[string]state.StorageConstraints) (*state.Application, error) {
		return s.State.AddApplication(state.AddApplicationArgs{Name: "storage-block2", Charm: ch, Storage: storage})
	}
	assertErr := func(storage map[string]state.StorageConstraints, expect string) {
		_, err := addService(storage)
		c.Assert(err, gc.ErrorMatches, expect)
	}

	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop-pool", 1024, 1),
		"multi2up":   makeStorageCons("loop-pool", 2048, 1),
	}
	assertErr(storageCons, `cannot add application "storage-block2": charm "storage-block2" store "multi2up": 2 instances required, 1 specified`)
	storageCons["multi2up"] = makeStorageCons("loop-pool", 1024, 2)
	assertErr(storageCons, `cannot add application "storage-block2": charm "storage-block2" store "multi2up": minimum storage size is 2.0GB, 1.0GB specified`)
	storageCons["multi2up"] = makeStorageCons("loop-pool", 2048, 2)
	storageCons["multi1to10"] = makeStorageCons("loop-pool", 1024, 11)
	assertErr(storageCons, `cannot add application "storage-block2": charm "storage-block2" store "multi1to10": at most 10 instances supported, 11 specified`)
	storageCons["multi1to10"] = makeStorageCons("ebs-fast", 1024, 10)
	assertErr(storageCons, `cannot add application "storage-block2": pool "ebs-fast" not found`)
	storageCons["multi1to10"] = makeStorageCons("loop-pool", 1024, 10)
	_, err := addService(storageCons)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuite) assertAddServiceStorageConstraintsDefaults(c *gc.C, pool string, cons, expect map[string]state.StorageConstraints) {
	if pool != "" {
		err := s.State.UpdateModelConfig(map[string]interface{}{
			"storage-default-block-source": pool,
		}, nil)
		c.Assert(err, jc.ErrorIsNil)
	}
	ch := s.AddTestingCharm(c, "storage-block")
	app, err := s.State.AddApplication(state.AddApplicationArgs{Name: "storage-block2", Charm: ch, Storage: cons})
	c.Assert(err, jc.ErrorIsNil)
	savedCons, err := app.StorageConstraints()
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
	app, err := s.State.AddApplication(state.AddApplicationArgs{Name: "storage-block2", Charm: ch, Storage: storageCons})
	c.Assert(err, jc.ErrorIsNil)
	savedCons, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCons, jc.DeepEquals, expectedCons)
}

func (s *StorageStateSuite) TestProviderFallbackToType(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	addService := func(storage map[string]state.StorageConstraints) (*state.Application, error) {
		return s.State.AddApplication(state.AddApplicationArgs{Name: "storage-block", Charm: ch, Storage: storage})
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
	err := s.State.UpdateModelConfig(map[string]interface{}{
		"storage-default-block-source": "loop-pool",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Each unit added to the application will create storage instances
	// to satisfy the application's storage constraints.
	ch := s.AddTestingCharm(c, "storage-block2")
	storage := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("", 1024, 1),
		"multi2up":   makeStorageCons("loop-pool", 2048, 2),
	}
	app := s.AddTestingServiceWithStorage(c, "storage-block2", ch, storage)
	for i := 0; i < 2; i++ {
		u, err := app.AddUnit(state.AddUnitParams{})
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
		owner, ok := one.Owner()
		c.Assert(ok, jc.IsTrue)
		c.Assert(ownerSet.Contains(owner.String()), jc.IsTrue)
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
	s.provisionStorageVolume(c, u, storageTag)

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
	err = s.State.DetachStorage(storageTag, u.UnitTag())
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
	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)

	si, err := s.State.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Life(), gc.Equals, state.Dying)

	err = s.State.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestRemoveStorageAttachmentsDisownsUnitOwnedInstance(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "persistent-block")

	si, err := s.State.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Life(), gc.Equals, state.Alive)

	// Assign the unit to a machine to create the volume and
	// volume attachment. When the storage is detached from
	// the unit, the volume should be detached from the
	// machine.
	err = s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	machineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(machineId)

	// Detaching the storage from the unit will leave the storage
	// behind, but will clear the ownership.
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	si, err = s.State.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	_, hasOwner := si.Owner()
	c.Assert(hasOwner, jc.IsFalse)

	// The volume should still be alive, but the attachment should be dying.
	volume := s.storageInstanceVolume(c, storageTag)
	c.Assert(volume.Life(), gc.Equals, state.Alive)
	volumeAttachment := s.volumeAttachment(c, machineTag, volume.VolumeTag())
	c.Assert(volumeAttachment.Life(), gc.Equals, state.Dying)
}

func (s *StorageStateSuite) TestAttachStorageTakesOwnership(c *gc.C) {
	app, u, storageTag := s.setupSingleStorageDetachable(c, "block", "modelscoped")
	u2, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Detach, but do not destroy, the storage.
	err = s.State.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	// Now attach the storage to the second unit.
	err = s.State.AttachStorage(storageTag, u2.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	storageInstance, err := s.State.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	owner, hasOwner := storageInstance.Owner()
	c.Assert(hasOwner, jc.IsTrue)
	c.Assert(owner, gc.Equals, u2.Tag())
}

func (s *StorageStateSuite) TestAttachStorageAssignedMachine(c *gc.C) {
	app, u, storageTag := s.setupSingleStorageDetachable(c, "block", "modelscoped")
	u2, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Detach, but do not destroy, the storage.
	err = s.State.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	// Assign the second unit to a machine so that when we
	// attach the storage to the unit, it will create a volume
	// and volume attachment.
	defer state.SetBeforeHooks(c, s.State, func() {
		err = s.State.AssignUnit(u2, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	// Now attach the storage to the second unit. There should now be a
	// volume and volume attachment.
	err = s.State.AttachStorage(storageTag, u2.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	volume := s.storageInstanceVolume(c, storageTag)
	machineId, err := u2.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(machineId)
	s.volumeAttachment(c, machineTag, volume.VolumeTag())
}

func (s *StorageStateSuite) TestAttachStorageAssignedMachineExistingVolume(c *gc.C) {
	// Create volume-backed filesystem storage.
	app, u, storageTag := s.setupSingleStorageDetachable(c, "filesystem", "modelscoped-block")
	u2, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Assign the first unit to a machine so that we have a
	// volume and volume attachment, and filesystem and
	// filesystem attachment initially. When we detach
	// the storage from the first unit, the volume and
	// filesystem should be detached from their assigned
	// machine.
	err = s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	oldMachineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	oldMachineTag := names.NewMachineTag(oldMachineId)
	volume := s.storageInstanceVolume(c, storageTag)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	// Detach, but do not destroy, the storage.
	err = s.State.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveFilesystemAttachment(oldMachineTag, filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = im.RemoveVolumeAttachment(oldMachineTag, volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)

	// Assign the second unit to a machine so that when we
	// attach the storage to the unit, it will attach the
	// existing volume/filesystem to the machine.
	defer state.SetBeforeHooks(c, s.State, func() {
		err = s.State.AssignUnit(u2, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	// Now attach the storage to the second unit. This should attach
	// the existing volume to the unit's machine.
	err = s.State.AttachStorage(storageTag, u2.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	machineId, err := u2.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(machineId)
	s.volumeAttachment(c, machineTag, volume.VolumeTag())
	s.filesystemAttachment(c, machineTag, filesystem.FilesystemTag())
}

func (s *StorageStateSuite) TestAttachStorageAssignedMachineExistingVolumeAttached(c *gc.C) {
	app, u, storageTag := s.setupSingleStorageDetachable(c, "block", "modelscoped")
	u2, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Assign the first unit to a machine so that we have a
	// volume and volume attachment initially. When we detach
	// the storage from the first unit, the volume should be
	// detached from its assigned machine.
	err = s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	// Assign the second unit to a machine so that when we
	// attach the storage to the unit, it will attach the
	// existing volume to the machine.
	err = s.State.AssignUnit(u2, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	// Detach, but do not destroy, the storage. Leave the volume attachment
	// in the model to show that we cannot attach the storage instance to
	// another unit/machine until it's gone.
	err = s.State.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.AttachStorage(storageTag, u2.UnitTag())
	c.Assert(err, gc.ErrorMatches,
		`cannot attach storage data/0 to unit quantal-storage-block/1: volume 0 is attached to machine 0`,
	)
}

func (s *StorageStateSuite) TestAddApplicationAttachStorage(c *gc.C) {
	app, u, storageTag := s.setupSingleStorageDetachable(c, "block", "modelscoped")

	// Detach, but do not destroy, the storage.
	err := s.State.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	ch, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	app2, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:   "secondwind",
		Series: app.Series(),
		Charm:  ch,
		Storage: map[string]state.StorageConstraints{
			// The unit should have two storage instances
			// in total. We're attaching one, so only one
			// new instance should be created.
			"data": makeStorageCons("modelscoped", 1024, 2),
		},
		AttachStorage: []names.StorageTag{storageTag},
		NumUnits:      1,
	})
	c.Assert(err, jc.ErrorIsNil)
	app2Units, err := app2.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app2Units, gc.HasLen, 1)

	// The storage instance should be attached to the new application unit.
	storageInstance, err := s.State.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	owner, hasOwner := storageInstance.Owner()
	c.Assert(hasOwner, jc.IsTrue)
	c.Assert(owner, gc.Equals, app2Units[0].UnitTag())
	storageAttachments, err := s.State.UnitStorageAttachments(app2Units[0].UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 2)
}

func (s *StorageStateSuite) TestAddApplicationAttachStorageMultipleUnits(c *gc.C) {
	app, _, storageTag := s.setupSingleStorageDetachable(c, "block", "modelscoped")
	ch, _, _ := app.Charm()
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:          "secondwind",
		Series:        app.Series(),
		Charm:         ch,
		AttachStorage: []names.StorageTag{storageTag},
		NumUnits:      2,
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application "secondwind": AttachStorage is non-empty but NumUnits is 2, must be 1`)
}

func (s *StorageStateSuite) TestAddApplicationAttachStorageTooMany(c *gc.C) {
	app, _, _ := s.setupSingleStorageDetachable(c, "block", "modelscoped")

	// Create 3 units whose storage instances we'll detach,
	// in order to attach to the new application below. The
	// charm allows a maximum of 2 storage instances, so the
	// application creation should fail.
	var storageTags []names.StorageTag
	for i := 0; i < 3; i++ {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		storageTag := names.NewStorageTag("data/" + fmt.Sprint(i+1))
		storageTags = append(storageTags, storageTag)

		// Detach, but do not destroy, the storage.
		err = s.State.DetachStorage(storageTag, u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
	}

	ch, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name:   "secondwind",
		Series: app.Series(),
		Charm:  ch,
		Storage: map[string]state.StorageConstraints{
			// The unit should have two storage instances
			// in total. We're attaching one, so only one
			// new instance should be created.
			"data": makeStorageCons("modelscoped", 1024, 2),
		},
		AttachStorage: storageTags,
		NumUnits:      1,
	})
	c.Assert(err, gc.ErrorMatches,
		`cannot add application "secondwind": `+
			`attaching 3 storage instances brings the total to 3, exceeding the maximum of 2`)
}

func (s *StorageStateSuite) TestAddUnitAttachStorage(c *gc.C) {
	app, u, storageTag := s.setupSingleStorageDetachable(c, "block", "modelscoped")

	// Detach, but do not destroy, the storage.
	err := s.State.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	// Add a new unit, attaching the existing storage.
	u2, err := app.AddUnit(state.AddUnitParams{
		AttachStorage: []names.StorageTag{storageTag},
	})
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance should be attached to the new application unit.
	storageInstance, err := s.State.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	owner, hasOwner := storageInstance.Owner()
	c.Assert(hasOwner, jc.IsTrue)
	c.Assert(owner, gc.Equals, u2.UnitTag())
}

func (s *StorageStateSuite) TestConcurrentDestroyStorageInstanceRemoveStorageAttachmentsRemovesInstance(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.DetachStorage(storageTag, u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	// Destroying the instance should check that there are no concurrent
	// changes to the storage instance's attachments, and recompute
	// operations if there are.
	err = s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)

	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestConcurrentRemoveStorageAttachment(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	s.provisionStorageVolume(c, u, storageTag)

	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)

	destroy := func() {
		err = s.State.DetachStorage(storageTag, u.UnitTag())
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
	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)

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
	err = s.State.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestConcurrentDestroyStorageInstance(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.DestroyStorageInstance(storageTag)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = s.State.DestroyStorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)

	si, err := s.State.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Life(), gc.Equals, state.Dying)
}

func (s *StorageStateSuite) TestDestroyStorageInstanceNotFound(c *gc.C) {
	err := s.State.DestroyStorageInstance(names.NewStorageTag("foo/0"))
	c.Assert(err, gc.ErrorMatches, `cannot destroy storage "foo/0": storage instance "foo/0" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *StorageStateSuite) TestWatchStorageAttachments(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block2")
	storage := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop-pool", 1024, 2),
		"multi2up":   makeStorageCons("loop-pool", 2048, 2),
	}
	app := s.AddTestingServiceWithStorage(c, "storage-block2", ch, storage)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	w := s.State.WatchStorageAttachments(u.UnitTag())
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange("multi1to10/0", "multi1to10/1", "multi2up/2", "multi2up/3")
	wc.AssertNoChange()

	err = s.State.DetachStorage(names.NewStorageTag("multi1to10/1"), u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("multi1to10/1")
	wc.AssertNoChange()
}

func (s *StorageStateSuite) TestWatchStorageAttachment(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	// Assign the unit to a machine, and provision the attachment. This
	// is necessary to prevent short-circuit removal of the attachment,
	// so that we can observe the progression from Alive->Dying->Dead->removed.
	s.provisionStorageVolume(c, u, storageTag)

	w := s.State.WatchStorageAttachment(storageTag, u.UnitTag())
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DetachStorage(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	err = s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *StorageStateSuite) TestDestroyUnitStorageAttachments(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "block")
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.DestroyUnitStorageAttachments(u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		attachments, err := s.State.UnitStorageAttachments(u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(attachments, gc.HasLen, 4)
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

	u1, err := svc1.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(u1, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	machineId, err := u1.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	u2, err := svc2.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u2.AssignToMachine(m)
	if expectErr == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, expectErr)
	}
}

func mustStorageConfig(name string, provider storage.ProviderType, attrs map[string]interface{}) *storage.Config {
	cfg, err := storage.NewConfig(name, provider, attrs)
	if err != nil {
		panic(err)
	}
	return cfg
}

var testingStorageProviders = storage.StaticProviderRegistry{
	map[storage.ProviderType]storage.Provider{
		"dummy": &dummystorage.StorageProvider{
			DefaultPools_: []*storage.Config{radiancePool},
		},
		"lancashire": &dummystorage.StorageProvider{
			DefaultPools_: []*storage.Config{blackPool},
		},
	},
}

var radiancePool = mustStorageConfig("radiance", "dummy", map[string]interface{}{"k": "v"})
var blackPool = mustStorageConfig("black", "lancashire", map[string]interface{}{})

func (s *StorageStateSuite) TestNewModelDefaultPools(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		StorageProviderRegistry: testingStorageProviders,
	})
	s.AddCleanup(func(*gc.C) { st.Close() })

	// When a model is created, it is populated with the default
	// pools of each storage provider supported by the model's
	// cloud provider.
	pm := poolmanager.New(state.NewStateSettings(st), testingStorageProviders)
	listed, err := pm.List()
	c.Assert(err, jc.ErrorIsNil)
	sort.Sort(byStorageConfigName(listed))
	c.Assert(listed, jc.DeepEquals, []*storage.Config{blackPool, radiancePool})
}

type byStorageConfigName []*storage.Config

func (c byStorageConfigName) Len() int {
	return len(c)
}

func (c byStorageConfigName) Less(a, b int) bool {
	return c[a].Name() < c[b].Name()
}

func (c byStorageConfigName) Swap(a, b int) {
	c[a], c[b] = c[b], c[a]
}

// TODO(axw) the following require shared storage support to test:
// - StorageAttachments can't be added to Dying StorageInstance
// - StorageInstance without attachments is removed by Destroy
// - concurrent add-unit and StorageAttachment removal does not
//   remove storage instance.

type StorageSubordinateStateSuite struct {
	StorageStateSuiteBase

	mysql                  *state.Application
	mysqlUnit              *state.Unit
	mysqlRelunit           *state.RelationUnit
	subordinateApplication *state.Application
	relation               *state.Relation
}

var _ = gc.Suite(&StorageSubordinateStateSuite{})

func (s *StorageSubordinateStateSuite) SetUpTest(c *gc.C) {
	s.StorageStateSuiteBase.SetUpTest(c)

	var err error
	storageCharm := s.AddTestingCharm(c, "storage-filesystem-subordinate")
	s.subordinateApplication = s.AddTestingService(c, "storage-filesystem-subordinate", storageCharm)
	s.mysql = s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.mysqlUnit, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	eps, err := s.State.InferEndpoints("mysql", "storage-filesystem-subordinate")
	c.Assert(err, jc.ErrorIsNil)
	s.relation, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	s.mysqlRelunit, err = s.relation.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageSubordinateStateSuite) TestSubordinateStoragePrincipalUnassigned(c *gc.C) {
	storageTag := names.NewStorageTag("data/0")
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)

	err := s.mysqlRelunit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// The subordinate unit will have been created, along with its storage.
	exists = s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsTrue)

	// The principal unit is not yet assigned to a machine, so there should
	// be no filesystem associated with the storage instance yet.
	_, err = s.State.StorageInstanceFilesystem(storageTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Assigning the principal unit to a machine should cause the subordinate
	// unit's machine storage to be created.
	err = s.State.AssignUnit(s.mysqlUnit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	_ = s.storageInstanceFilesystem(c, storageTag)
}

func (s *StorageSubordinateStateSuite) TestSubordinateStoragePrincipalAssigned(c *gc.C) {
	err := s.State.AssignUnit(s.mysqlUnit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysqlRelunit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// The subordinate unit will have been created, along with its storage.
	storageTag := names.NewStorageTag("data/0")
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsTrue)

	// The principal unit was assigned to a machine when the subordinate
	// unit was created, so there should be a filesystem associated with
	// the storage instance now.
	_ = s.storageInstanceFilesystem(c, storageTag)
}

func (s *StorageSubordinateStateSuite) TestSubordinateStoragePrincipalAssignRace(c *gc.C) {
	// Add the subordinate before attempting to commit the transaction
	// that assigns the unit to a machine. The transaction should fail
	// and be reattempted with the knowledge of the subordinate, and
	// add the subordinate's storage.
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.mysqlRelunit.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := s.State.AssignUnit(s.mysqlUnit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	_ = s.storageInstanceFilesystem(c, names.NewStorageTag("data/0"))
}
