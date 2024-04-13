// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/charm/v13"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/provider/caas"
	k8sprovider "github.com/juju/juju/internal/provider/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/internal/provider/caas/kubernetes/provider/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/testing"
)

type StorageStateSuiteBase struct {
	ConnSuite

	series         string
	base           state.Base
	st             *state.State
	storageBackend *state.StorageBackend
}

func (s *StorageStateSuiteBase) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.PatchValue(&k8sprovider.NewK8sClients, k8stesting.NoopFakeK8sClients)

	var registry storage.ProviderRegistry
	if s.series == "focal" {
		s.base = state.UbuntuBase("20.04")
		s.st = s.Factory.MakeCAASModel(c, nil)
		s.AddCleanup(func(_ *gc.C) { s.st.Close() })
		var err error
		s.Model, err = s.st.Model()
		c.Assert(err, jc.ErrorIsNil)
		broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(
			s.Model,
			&testing.MockCloudService{&cloud.Cloud{Name: "caascloud", Type: "kubernetes"}},
			&testing.MockCredentialService{ptr(cloud.NewCredential(cloud.UserPassAuthType, nil))},
		)
		c.Assert(err, jc.ErrorIsNil)
		registry = stateenvirons.NewStorageProviderRegistry(broker)
	} else {
		s.series = "quantal"
		s.base = state.UbuntuBase("12.10")
		s.st = s.State
		registry = storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	}
	s.policy = testing.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return registry, nil
		},
	}

	// Create a default pool for block devices.
	s.policy.Providers = map[string]domainstorage.StoragePoolDetails{
		"loop-pool":  {Name: "loop-pool", Provider: "loop"},
		"tmpfs-pool": {Name: "tmpfs-pool", Provider: "tmpfs"},
	}

	if s.series != "focal" {
		// Create a pool that creates persistent block devices.
		s.policy.Providers["persistent-block"] = domainstorage.StoragePoolDetails{
			Name:     "persistent-block",
			Provider: "modelscoped-block",
			Attrs: map[string]string{
				"persistent": "true",
			},
		}
	} else {
		// Create the operator-storage
		s.policy.Providers["k8s-operator-storage"] = domainstorage.StoragePoolDetails{
			Name:     "k8s-operator-storage",
			Provider: "loop",
		}
	}

	var err error
	s.storageBackend, err = state.NewStorageBackend(s.st)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) AddTestingCharm(c *gc.C, name string) *state.Charm {
	return state.AddTestingCharmForSeries(c, s.st, s.series, name)
}

func (s *StorageStateSuiteBase) AddTestingApplication(c *gc.C, name string, ch *state.Charm) *state.Application {
	return state.AddTestingApplicationForBase(c, s.st, s.objectStore, s.base, name, ch)
}

func (s *StorageStateSuiteBase) AddTestingApplicationWithStorage(c *gc.C, name string, ch *state.Charm, storage map[string]state.StorageConstraints) *state.Application {
	return state.AddTestingApplicationWithStorage(c, s.st, s.objectStore, name, ch, storage)
}

func (s *StorageStateSuiteBase) AddMetaCharm(c *gc.C, name, metaYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.st, name, "metadata.yaml", metaYaml, s.series, revision)
}

func (s *StorageStateSuiteBase) setupSingleStorage(c *gc.C, kind, pool string) (*state.Application, *state.Unit, names.StorageTag) {
	// There are test charms called "storage-block" and
	// "storage-filesystem" which are what you'd expect.
	ch := s.AddTestingCharm(c, "storage-"+kind)
	testStorage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	app := s.AddTestingApplicationWithStorage(c, "storage-"+kind, ch, testStorage)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	storageTag := names.NewStorageTag("data/0")
	return app, unit, storageTag
}

func (s *StorageStateSuiteBase) provisionStorageVolume(c *gc.C, u *state.Unit, storageTag names.StorageTag) {
	err := s.st.AssignUnit(defaultInstancePrechecker, u, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)
	machine := unitMachine(c, s.st, u)
	volume := s.storageInstanceVolume(c, storageTag)
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{VolumeId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeAttachmentInfo(
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
	testStorage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	app := s.AddTestingApplicationWithStorage(c, charm.MustParseURL(ch.URL()).Name, ch, testStorage)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	storageTag := names.NewStorageTag("data/0")
	return app, unit, storageTag
}

func (s *StorageStateSuiteBase) createStorageCharmWithSeries(c *gc.C, charmName string, storageMeta charm.Storage, series string) *state.Charm {
	return s.createStorageCharmRevMeta(c, charmName, storageMeta, 1, fmt.Sprintf("series: [%s]", series))
}

func (s *StorageStateSuiteBase) createStorageCharm(c *gc.C, charmName string, storageMeta charm.Storage) *state.Charm {
	return s.createStorageCharmRev(c, charmName, storageMeta, 1)
}

func (s *StorageStateSuiteBase) createStorageCharmRev(c *gc.C, charmName string, storageMeta charm.Storage, rev int) *state.Charm {
	return s.createStorageCharmRevMeta(c, charmName, storageMeta, rev, "")
}

func (s *StorageStateSuiteBase) createStorageCharmRevMeta(c *gc.C, charmName string, storageMeta charm.Storage, rev int, additionalMeta string) *state.Charm {
	meta := fmt.Sprintf(`
name: %s
summary: A charm for testing storage
description: ditto
%s
storage:
  %s:
    type: %s
`, charmName, additionalMeta, storageMeta.Name, storageMeta.Type)
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
	return s.AddTestingApplicationWithStorage(c, "storage-"+kind+"2", ch, storageCons)
}

func (s *StorageStateSuiteBase) storageInstanceExists(c *gc.C, tag names.StorageTag) bool {
	_, err := state.TxnRevno(
		s.st,
		state.StorageInstancesC,
		state.DocID(s.st, tag.Id()),
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
	c.Assert(err, jc.ErrorIs, errors.NotProvisioned)
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

func (s *StorageStateSuiteBase) assertFilesystemAttachmentUnprovisioned(c *gc.C, host names.Tag, f names.FilesystemTag) {
	filesystemAttachment := s.filesystemAttachment(c, host, f)
	_, err := filesystemAttachment.Info()
	c.Assert(err, jc.ErrorIs, errors.NotProvisioned)
	_, ok := filesystemAttachment.Params()
	c.Assert(ok, jc.IsTrue)
}

func (s *StorageStateSuiteBase) assertFilesystemAttachmentInfo(c *gc.C, host names.Tag, f names.FilesystemTag, expect state.FilesystemAttachmentInfo) {
	filesystemAttachment := s.filesystemAttachment(c, host, f)
	info, err := filesystemAttachment.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expect)
	_, ok := filesystemAttachment.Params()
	c.Assert(ok, jc.IsFalse)
}

func (s *StorageStateSuiteBase) assertVolumeUnprovisioned(c *gc.C, tag names.VolumeTag) {
	volume := s.volume(c, tag)
	_, err := volume.Info()
	c.Assert(err, jc.ErrorIs, errors.NotProvisioned)
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
	filesystem, err := s.storageBackend.Filesystem(tag)
	c.Assert(err, jc.ErrorIsNil)
	return filesystem
}

func (s *StorageStateSuiteBase) filesystemVolume(c *gc.C, tag names.FilesystemTag) state.Volume {
	filesystem := s.filesystem(c, tag)
	volumeTag, err := filesystem.Volume()
	c.Assert(err, jc.ErrorIsNil)
	return s.volume(c, volumeTag)
}

func (s *StorageStateSuiteBase) filesystemAttachment(c *gc.C, host names.Tag, f names.FilesystemTag) state.FilesystemAttachment {
	attachment, err := s.storageBackend.FilesystemAttachment(host, f)
	c.Assert(err, jc.ErrorIsNil)
	return attachment
}

func (s *StorageStateSuiteBase) volume(c *gc.C, tag names.VolumeTag) state.Volume {
	volume, err := s.storageBackend.Volume(tag)
	c.Assert(err, jc.ErrorIsNil)
	return volume
}

func (s *StorageStateSuiteBase) volumeFilesystem(c *gc.C, tag names.VolumeTag) state.Filesystem {
	filesystem, err := s.storageBackend.VolumeFilesystem(tag)
	c.Assert(err, jc.ErrorIsNil)
	return filesystem
}

func (s *StorageStateSuiteBase) volumeAttachment(c *gc.C, host names.Tag, v names.VolumeTag) state.VolumeAttachment {
	attachment, err := s.storageBackend.VolumeAttachment(host, v)
	c.Assert(err, jc.ErrorIsNil)
	return attachment
}

func (s *StorageStateSuiteBase) volumeAttachmentPlan(c *gc.C, host names.Tag, v names.VolumeTag) state.VolumeAttachmentPlan {
	attachmentPlan, err := s.storageBackend.VolumeAttachmentPlan(host, v)
	c.Assert(err, jc.ErrorIsNil)
	return attachmentPlan
}

func (s *StorageStateSuiteBase) storageInstanceVolume(c *gc.C, tag names.StorageTag) state.Volume {
	volume, err := s.storageBackend.StorageInstanceVolume(tag)
	c.Assert(err, jc.ErrorIsNil)
	return volume
}

func (s *StorageStateSuiteBase) storageInstanceFilesystem(c *gc.C, tag names.StorageTag) state.Filesystem {
	filesystem, err := s.storageBackend.StorageInstanceFilesystem(tag)
	c.Assert(err, jc.ErrorIsNil)
	return filesystem
}

func (s *StorageStateSuiteBase) obliterateUnit(c *gc.C, tag names.UnitTag) {
	u, err := s.st.Unit(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	s.obliterateUnitStorage(c, tag)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) obliterateUnitStorage(c *gc.C, tag names.UnitTag) {
	attachments, err := s.storageBackend.UnitStorageAttachments(tag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		err = s.storageBackend.DetachStorage(a.StorageInstance(), a.Unit(), false, dontWait)
		c.Assert(err, jc.ErrorIsNil)
		if _, err := s.storageBackend.StorageAttachment(a.StorageInstance(), a.Unit()); err == nil {
			err = s.storageBackend.RemoveStorageAttachment(a.StorageInstance(), a.Unit(), false)
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func (s *StorageStateSuiteBase) obliterateVolume(c *gc.C, tag names.VolumeTag) {
	err := s.storageBackend.DestroyVolume(tag, false)
	if errors.Is(err, errors.NotFound) {
		return
	}
	attachments, err := s.storageBackend.VolumeAttachments(tag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		s.obliterateVolumeAttachment(c, a.Host(), a.Volume())
	}
	err = s.storageBackend.RemoveVolume(tag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) obliterateVolumeAttachment(c *gc.C, m names.Tag, v names.VolumeTag) {
	err := s.storageBackend.DetachVolume(m, v, false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(m, v, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) obliterateFilesystem(c *gc.C, tag names.FilesystemTag) {
	err := s.storageBackend.DestroyFilesystem(tag, false)
	if errors.Is(err, errors.NotFound) {
		return
	}
	attachments, err := s.storageBackend.FilesystemAttachments(tag)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range attachments {
		s.obliterateFilesystemAttachment(c, a.Host(), a.Filesystem())
	}
	err = s.storageBackend.RemoveFilesystem(tag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuiteBase) obliterateFilesystemAttachment(c *gc.C, host names.Tag, f names.FilesystemTag) {
	err := s.storageBackend.DetachFilesystem(host, f)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(host, f, false)
	c.Assert(err, jc.ErrorIsNil)
}

// assertMachineStorageRefs ensures that the specified machine's set of volume
// and filesystem references corresponds exactly to the volume and filesystem
// attachments that relate to the machine.
func assertMachineStorageRefs(c *gc.C, sb *state.StorageBackend, m names.MachineTag) {
	mb := state.ModelBackendFromStorageBackend(sb)
	machines, closer := state.GetRawCollection(mb, state.MachinesC)
	defer closer()

	var doc struct {
		Volumes     []string `bson:"volumes,omitempty"`
		Filesystems []string `bson:"filesystems,omitempty"`
	}
	err := machines.FindId(state.DocID(mb, m.Id())).One(&doc)
	c.Assert(err, jc.ErrorIsNil)

	have := names.NewSet()
	for _, v := range doc.Volumes {
		have.Add(names.NewVolumeTag(v))
	}
	for _, f := range doc.Filesystems {
		have.Add(names.NewFilesystemTag(f))
	}

	expect := names.NewSet()
	volumeAttachments, err := sb.MachineVolumeAttachments(m)
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range volumeAttachments {
		expect.Add(a.Volume())
	}
	filesystemAttachments, err := sb.MachineFilesystemAttachments(m)
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

func (s *StorageStateSuite) TestBlockStorageNotSupportedOnCAAS(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	ch := state.AddTestingCharmForSeries(c, st, "focal", "storage-block")
	_, err := st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "storage-block", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
	}, state.NewObjectStore(c, st.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "storage-block": block storage on a container model not supported`)
}

func (s *StorageStateSuite) TestAddApplicationStorageConstraintsDefault(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storageBlock, err := s.st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "storage-block", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.st.ModelUUID()))
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
	storageFilesystem, err := s.st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "storage-filesystem", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}, state.NewObjectStore(c, s.st.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	constraints, err = storageFilesystem.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	defaultStorage := "rootfs"
	if s.series == "focal" {
		defaultStorage = "kubernetes"
	}
	c.Assert(constraints, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Pool:  defaultStorage,
			Count: 1,
			Size:  1024,
		},
	})
}

func (s *StorageStateSuite) TestAddApplicationStorageConstraintsValidation(c *gc.C) {
	s.policy.Providers = map[string]domainstorage.StoragePoolDetails{
		"loop-pool": {Name: "loop-pool", Provider: "loop"},
	}

	ch := s.AddTestingCharm(c, "storage-block2")
	addApplication := func(storage map[string]state.StorageConstraints) (*state.Application, error) {
		return s.st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
			Name: "storage-block2", Charm: ch,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "22.04/stable",
			}},
			Storage: storage,
		}, state.NewObjectStore(c, s.st.ModelUUID()))
	}
	assertErr := func(storage map[string]state.StorageConstraints, expect string) {
		_, err := addApplication(storage)
		c.Assert(err, gc.ErrorMatches, expect)
	}

	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop-pool", 1024, 1),
		"multi2up":   makeStorageCons("loop-pool", 2048, 1),
	}
	assertErr(storageCons, `cannot add application "storage-block2": charm "storage-block2" store "multi2up": 2 instances required, 1 specified`)
	storageCons["multi2up"] = makeStorageCons("loop-pool", 1024, 2)
	assertErr(storageCons, `cannot add application "storage-block2": charm "storage-block2" store "multi2up": minimum storage size is 2.0 GB, 1.0 GB specified`)
	storageCons["multi2up"] = makeStorageCons("loop-pool", 2048, 2)
	storageCons["multi1to10"] = makeStorageCons("loop-pool", 1024, 11)
	assertErr(storageCons, `cannot add application "storage-block2": charm "storage-block2" store "multi1to10": at most 10 instances supported, 11 specified`)
	storageCons["multi1to10"] = makeStorageCons("ebs-fast", 1024, 10)
	assertErr(storageCons, `cannot add application "storage-block2": storage pool "ebs-fast" not found`)
	storageCons["multi1to10"] = makeStorageCons("loop-pool", 1024, 10)
	_, err := addApplication(storageCons)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuite) assertAddApplicationStorageConstraintsDefaults(c *gc.C, pool string, cons, expect map[string]state.StorageConstraints) {
	if pool != "" {
		err := s.Model.UpdateModelConfig(state.NoopConfigSchemaSource, map[string]interface{}{
			"storage-default-block-source": pool,
		}, nil)
		c.Assert(err, jc.ErrorIsNil)
	}
	ch := s.AddTestingCharm(c, "storage-block")
	app, err := s.st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "storage-block2", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		Storage: cons,
	}, state.NewObjectStore(c, s.st.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	savedCons, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCons, jc.DeepEquals, expect)
	// TODO(wallyworld) - test pool name stored in data model
}

func (s *StorageStateSuite) TestAddApplicationStorageConstraintsNoConstraintsUsed(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 0, 0),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data":    makeStorageCons("loop", 1024, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddApplicationStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddApplicationStorageConstraintsJustCount(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 0, 1),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data":    makeStorageCons("loop-pool", 1024, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddApplicationStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddApplicationStorageConstraintsDefaultPool(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 2048, 1),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data":    makeStorageCons("loop-pool", 2048, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddApplicationStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddApplicationStorageConstraintsNoUserDefaultPool(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 2048, 1),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data":    makeStorageCons("loop", 2048, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddApplicationStorageConstraintsDefaults(c, "", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddApplicationStorageConstraintsDefaultSizeFallback(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop-pool", 0, 1),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data":    makeStorageCons("loop-pool", 1024, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddApplicationStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddApplicationStorageConstraintsDefaultSizeFromCharm(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop", 0, 3),
	}
	expectedCons := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop", 1024, 3),
		"multi2up":   makeStorageCons("loop", 2048, 2),
	}
	ch := s.AddTestingCharm(c, "storage-block2")
	app, err := s.st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "storage-block2", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		Storage: storageCons,
	}, state.NewObjectStore(c, s.st.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	savedCons, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCons, jc.DeepEquals, expectedCons)
}

func (s *StorageStateSuite) TestProviderFallbackToType(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	addApplication := func(storage map[string]state.StorageConstraints) (*state.Application, error) {
		return s.st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
			Name: "storage-block", Charm: ch,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "22.04/stable",
			}},
			Storage: storage,
		}, state.NewObjectStore(c, s.st.ModelUUID()))
	}
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop", 1024, 1),
	}
	_, err := addApplication(storageCons)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuite) TestAddUnit(c *gc.C) {
	s.assertStorageUnitsAdded(c)
}

func (s *StorageStateSuite) assertStorageUnitsAdded(c *gc.C) {
	err := s.Model.UpdateModelConfig(state.NoopConfigSchemaSource, map[string]interface{}{
		"storage-default-block-source": "loop-pool",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Each unit added to the application will create storage instances
	// to satisfy the application's storage constraints.
	ch := s.AddTestingCharm(c, "storage-block2")
	testStorage := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("", 1024, 1),
		"multi2up":   makeStorageCons("loop-pool", 2048, 2),
	}
	app := s.AddTestingApplicationWithStorage(c, "storage-block2", ch, testStorage)
	for i := 0; i < 2; i++ {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		storageAttachments, err := s.storageBackend.UnitStorageAttachments(u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		count := make(map[string]int)
		for _, att := range storageAttachments {
			c.Assert(att.Unit(), gc.Equals, u.UnitTag())
			storageInstance, err := s.storageBackend.StorageInstance(att.StorageInstance())
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

	all, err := s.storageBackend.AllStorageInstances()
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
		attachments, err := s.storageBackend.StorageAttachments(tag)
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
	all, err := s.storageBackend.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}

func (s *StorageStateSuite) TestUnitEnsureDead(c *gc.C) {
	if s.series == "focal" {
		c.Skip("volumes on containers not supported")
	}
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	s.provisionStorageVolume(c, u, storageTag)

	// destroying a unit with storage attachments is fine; this is what
	// will trigger the death and removal of storage attachments.
	err := u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	// until all storage attachments are removed, the unit cannot be
	// marked as being dead.
	assertUnitEnsureDeadError := func() {
		err = u.EnsureDead()
		c.Assert(err, gc.ErrorMatches, "unit has storage attachments")
	}
	assertUnitEnsureDeadError()
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	assertUnitEnsureDeadError()
	err = s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	assertUnitEnsureDeadError()
	err = s.storageBackend.RemoveStorageAttachment(storageTag, u.UnitTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuite) TestUnitStorageProvisionerError(c *gc.C) {
	if s.series == "focal" {
		c.Skip("volumes on containers not supported")
	}
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	s.provisionStorageVolume(c, u, storageTag)

	err := u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	// until all storage attachments are removed, the unit cannot be
	// marked as being dead.
	assertUnitEnsureDeadError := func() {
		err = u.EnsureDead()
		c.Assert(err, gc.ErrorMatches, "unit has storage attachments")
	}
	assertUnitEnsureDeadError()
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	assertUnitEnsureDeadError()
	err = s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	assertUnitEnsureDeadError()
	err = s.storageBackend.RemoveStorageAttachment(storageTag, u.UnitTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Removing a unit will still succeed even if the storage provisioner is broken.
	s.policy = testing.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return nil, errors.New("boom")
		},
	}
	err = u.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuite) TestRemoveStorageAttachmentsRemovesDyingInstance(c *gc.C) {
	if s.series == "focal" {
		c.Skip("volumes on containers not supported")
	}
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	// Mark the storage instance as Dying, so that it will be removed
	// when the last attachment is removed.
	err := u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	si, err := s.storageBackend.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Life(), gc.Equals, state.Dying)

	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestRemoveStorageAttachmentsDisownsUnitOwnedInstance(c *gc.C) {
	if s.series == "focal" {
		c.Skip("volumes on containers not supported")
	}
	_, u, storageTag := s.setupSingleStorage(c, "block", "persistent-block")

	si, err := s.storageBackend.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Life(), gc.Equals, state.Alive)

	// Assign the unit to a machine to create the volume and
	// volume attachment. When the storage is detached from
	// the unit, the volume should be detached from the
	// machine.
	err = s.st.AssignUnit(defaultInstancePrechecker, u, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)
	machineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(machineId)

	// Detaching the storage from the unit will leave the storage
	// behind, but will clear the ownership.
	err = u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	si, err = s.storageBackend.StorageInstance(storageTag)
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
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	// Now attach the storage to the second unit.
	err = s.storageBackend.AttachStorage(storageTag, u2.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	storageInstance, err := s.storageBackend.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	owner, hasOwner := storageInstance.Owner()
	c.Assert(hasOwner, jc.IsTrue)
	c.Assert(owner, gc.Equals, u2.Tag())
}

func (s *StorageStateSuite) TestAttachStorageIdempotent(c *gc.C) {
	app, u, storageTag := s.setupSingleStorageDetachable(c, "block", "modelscoped")
	u2, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Detach, but do not destroy, the storage.
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	// Now attach the storage to the second unit.
	err = s.storageBackend.AttachStorage(storageTag, u2.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	// And again.
	err = s.storageBackend.AttachStorage(storageTag, u2.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuite) TestAttachStorageAssignedMachine(c *gc.C) {
	app, u, storageTag := s.setupSingleStorageDetachable(c, "block", "modelscoped")
	u2, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Detach, but do not destroy, the storage.
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	// Assign the second unit to a machine so that when we
	// attach the storage to the unit, it will create a volume
	// and volume attachment.
	defer state.SetBeforeHooks(c, s.st, func() {
		err = s.st.AssignUnit(defaultInstancePrechecker, u2, state.AssignNew)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	// Now attach the storage to the second unit. There should now be a
	// volume and volume attachment.
	err = s.storageBackend.AttachStorage(storageTag, u2.UnitTag())
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
	err = s.st.AssignUnit(defaultInstancePrechecker, u, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)
	oldMachineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	oldMachineTag := names.NewMachineTag(oldMachineId)
	volume := s.storageInstanceVolume(c, storageTag)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	// Detach, but do not destroy, the storage.
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(oldMachineTag, filesystem.FilesystemTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(oldMachineTag, volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	// Assign the second unit to a machine so that when we
	// attach the storage to the unit, it will attach the
	// existing volume/filesystem to the machine.
	defer state.SetBeforeHooks(c, s.st, func() {
		err = s.st.AssignUnit(defaultInstancePrechecker, u2, state.AssignNew)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	// Now attach the storage to the second unit. This should attach
	// the existing volume to the unit's machine.
	err = s.storageBackend.AttachStorage(storageTag, u2.UnitTag())
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
	err = s.st.AssignUnit(defaultInstancePrechecker, u, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)

	// Assign the second unit to a machine so that when we
	// attach the storage to the unit, it will attach the
	// existing volume to the machine.
	err = s.st.AssignUnit(defaultInstancePrechecker, u2, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)

	// Detach, but do not destroy, the storage. Leave the volume attachment
	// in the model to show that we cannot attach the storage instance to
	// another unit/machine until it's gone.
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	err = s.storageBackend.AttachStorage(storageTag, u2.UnitTag())
	c.Assert(err, gc.ErrorMatches,
		`cannot attach storage data/0 to unit quantal-storage-block/1: volume 0 is attached to machine 0`,
	)
}

func (s *StorageStateSuite) TestAddApplicationAttachStorage(c *gc.C) {
	app, u, storageTag := s.setupSingleStorageDetachable(c, "block", "modelscoped")

	// Detach, but do not destroy, the storage.
	err := s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	ch, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	app2, err := s.st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "secondwind",
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm: ch,
		Storage: map[string]state.StorageConstraints{
			// The unit should have two storage instances
			// in total. We're attaching one, so only one
			// new instance should be created.
			"data": makeStorageCons("modelscoped", 1024, 2),
		},
		AttachStorage: []names.StorageTag{storageTag},
		NumUnits:      1,
	}, state.NewObjectStore(c, s.st.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	app2Units, err := app2.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app2Units, gc.HasLen, 1)

	// The storage instance should be attached to the new application unit.
	storageInstance, err := s.storageBackend.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	owner, hasOwner := storageInstance.Owner()
	c.Assert(hasOwner, jc.IsTrue)
	c.Assert(owner, gc.Equals, app2Units[0].UnitTag())
	storageAttachments, err := s.storageBackend.UnitStorageAttachments(app2Units[0].UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 2)
}

func (s *StorageStateSuite) TestAddApplicationAttachStorageMultipleUnits(c *gc.C) {
	app, _, storageTag := s.setupSingleStorageDetachable(c, "block", "modelscoped")
	ch, _, _ := app.Charm()
	_, err := s.st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "secondwind",
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm:         ch,
		AttachStorage: []names.StorageTag{storageTag},
		NumUnits:      2,
	}, state.NewObjectStore(c, s.st.ModelUUID()))
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
		err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
		c.Assert(err, jc.ErrorIsNil)
	}

	ch, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.st.AddApplication(defaultInstancePrechecker, state.AddApplicationArgs{
		Name: "secondwind",
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm: ch,
		Storage: map[string]state.StorageConstraints{
			// The unit should have two storage instances
			// in total. We're attaching one, so only one
			// new instance should be created.
			"data": makeStorageCons("modelscoped", 1024, 2),
		},
		AttachStorage: storageTags,
		NumUnits:      1,
	}, state.NewObjectStore(c, s.st.ModelUUID()))
	c.Assert(err, gc.ErrorMatches,
		`cannot add application "secondwind": `+
			`attaching 3 storage instances brings the total to 3, exceeding the maximum of 2`)
}

func (s *StorageStateSuite) TestAddUnitAttachStorage(c *gc.C) {
	app, u, storageTag := s.setupSingleStorageDetachable(c, "block", "modelscoped")

	// Detach, but do not destroy, the storage.
	err := s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	// Add a new unit, attaching the existing storage.
	u2, err := app.AddUnit(state.AddUnitParams{
		AttachStorage: []names.StorageTag{storageTag},
	})
	c.Assert(err, jc.ErrorIsNil)

	// The storage instance should be attached to the new application unit.
	storageInstance, err := s.storageBackend.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	owner, hasOwner := storageInstance.Owner()
	c.Assert(hasOwner, jc.IsTrue)
	c.Assert(owner, gc.Equals, u2.UnitTag())
}

func (s *StorageStateSuite) TestConcurrentDestroyStorageInstanceRemoveStorageAttachmentsRemovesInstance(c *gc.C) {
	if s.series == "focal" {
		c.Skip("volumes on containers not supported")
	}
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.st, func() {
		err := s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	// Destroying the instance should check that there are no concurrent
	// changes to the storage instance's attachments, and recompute
	// operations if there are.
	err = s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestConcurrentRemoveStorageAttachment(c *gc.C) {
	if s.series == "focal" {
		c.Skip("volumes on containers not supported")
	}
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	s.provisionStorageVolume(c, u, storageTag)

	err := u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	destroy := func() {
		err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
		c.Assert(err, jc.ErrorIsNil)
	}
	remove := func() {
		err = s.storageBackend.RemoveStorageAttachment(storageTag, u.UnitTag(), false)
		c.Assert(err, jc.ErrorIsNil)
	}

	defer state.SetBeforeHooks(c, s.st, destroy, remove).Check()
	destroy()
	remove()
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestRemoveAliveStorageAttachmentError(c *gc.C) {
	if s.series == "focal" {
		c.Skip("volumes on containers not supported")
	}
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	err := s.storageBackend.RemoveStorageAttachment(storageTag, u.UnitTag(), false)
	c.Assert(err, gc.ErrorMatches, "cannot remove storage attachment data/0:storage-block/0: storage attachment is not dying")

	attachments, err := s.storageBackend.UnitStorageAttachments(u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 1)
	c.Assert(attachments[0].StorageInstance(), gc.Equals, storageTag)
}

func (s *StorageStateSuite) TestConcurrentDestroyInstanceRemoveStorageAttachmentsRemovesInstance(c *gc.C) {
	if s.series == "focal" {
		c.Skip("volumes on containers not supported")
	}
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.st, func() {
		// Concurrently mark the storage instance as Dying,
		// so that it will be removed when the last attachment
		// is removed.
		err := s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
		c.Assert(err, jc.ErrorIsNil)
	}, nil).Check()

	// Removing the attachment should check that there are no concurrent
	// changes to the storage instance's life, and recompute operations
	// if it does.
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestConcurrentDestroyStorageInstance(c *gc.C) {
	if s.series == "focal" {
		c.Skip("volumes on containers not supported")
	}
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	err := u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.st, func() {
		err := s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)

	si, err := s.storageBackend.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Life(), gc.Equals, state.Dying)
}

func (s *StorageStateSuite) TestDestroyStorageInstanceNotFound(c *gc.C) {
	err := s.storageBackend.DestroyStorageInstance(names.NewStorageTag("foo/0"), true, false, dontWait)
	c.Assert(err, gc.ErrorMatches, `cannot destroy storage "foo/0": storage instance "foo/0" not found`)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *StorageStateSuite) TestDestroyStorageInstanceAttachedError(c *gc.C) {
	if s.series == "focal" {
		c.Skip("volumes on containers not supported")
	}
	_, _, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	err := s.storageBackend.DestroyStorageInstance(storageTag, false, false, dontWait)
	c.Assert(err, gc.ErrorMatches, `cannot destroy storage "data/0": storage is attached`)
	c.Assert(err, jc.ErrorIs, stateerrors.StorageAttachedError)
}

func (s *StorageStateSuite) TestWatchStorageAttachments(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block2")
	testStorage := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop-pool", 1024, 2),
		"multi2up":   makeStorageCons("loop-pool", 2048, 2),
	}
	app := s.AddTestingApplicationWithStorage(c, "storage-block2", ch, testStorage)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchStorageAttachments(u.UnitTag())
	defer workertest.CleanKill(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("multi1to10/0", "multi1to10/1", "multi2up/2", "multi2up/3")
	wc.AssertNoChange()

	err = s.storageBackend.DetachStorage(names.NewStorageTag("multi1to10/1"), u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("multi1to10/1")
	wc.AssertNoChange()
}

func (s *StorageStateSuite) TestWatchStorageAttachment(c *gc.C) {
	if s.series == "focal" {
		c.Skip("volumes on containers not supported")
	}
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")
	// Assign the unit to a machine, and provision the attachment. This
	// is necessary to prevent short-circuit removal of the attachment,
	// so that we can observe the progression from Alive->Dying->Dead->removed.
	s.provisionStorageVolume(c, u, storageTag)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchStorageAttachment(storageTag, u.UnitTag())
	defer workertest.CleanKill(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	err := u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	err = s.storageBackend.RemoveStorageAttachment(storageTag, u.UnitTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *StorageStateSuite) TestDestroyUnitStorageAttachments(c *gc.C) {
	app := s.setupMixedScopeStorageApplication(c, "block")
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.st, func() {
		err := s.storageBackend.DestroyUnitStorageAttachments(u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		attachments, err := s.storageBackend.UnitStorageAttachments(u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(attachments, gc.HasLen, 4)
		for _, a := range attachments {
			c.Assert(a.Life(), gc.Equals, state.Dying)
			err := s.storageBackend.RemoveStorageAttachment(a.StorageInstance(), u.UnitTag(), false)
			c.Assert(err, jc.ErrorIsNil)
		}
	}).Check()

	err = s.storageBackend.DestroyUnitStorageAttachments(u.UnitTag())
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
	app1 := s.AddTestingApplication(c, "storage-filesystem", ch1)
	app2 := s.AddTestingApplication(c, "storage-filesystem2", ch2)

	u1, err := app1.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.AssignUnit(defaultInstancePrechecker, u1, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)

	machineId, err := u1.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.st.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	u2, err := app2.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u2.AssignToMachine(m)
	if expectErr == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, expectErr)
	}
}

type StorageStateSuiteCaas struct {
	StorageStateSuiteBase
}

var _ = gc.Suite(&StorageStateSuiteCaas{})

func (s *StorageStateSuiteCaas) SetUpTest(c *gc.C) {
	// Use focal for k8s charms (quantal for machine charms).
	s.series = "focal"
	s.StorageStateSuiteBase.SetUpTest(c)
}

func (s *StorageStateSuiteCaas) TestDeployWrongStorageType(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	args := state.AddApplicationArgs{
		Name:  "foo",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		NumUnits: 1,
		Storage: map[string]state.StorageConstraints{
			"data": {Pool: "loop"},
		},
	}
	_, err := s.st.AddApplication(defaultInstancePrechecker, args, state.NewObjectStore(c, s.st.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "foo": invalid storage config: storage provider type "loop" not valid`)
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
	s.subordinateApplication = s.AddTestingApplication(c, "storage-filesystem-subordinate", storageCharm)
	s.mysql = s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.mysqlUnit, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	eps, err := s.st.InferEndpoints("mysql", "storage-filesystem-subordinate")
	c.Assert(err, jc.ErrorIsNil)
	s.relation, err = s.st.AddRelation(eps...)
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
	_, err = s.storageBackend.StorageInstanceFilesystem(storageTag)
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	// Assigning the principal unit to a machine should cause the subordinate
	// unit's machine storage to be created.
	err = s.st.AssignUnit(defaultInstancePrechecker, s.mysqlUnit, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)
	_ = s.storageInstanceFilesystem(c, storageTag)
}

func (s *StorageSubordinateStateSuite) TestSubordinateStoragePrincipalAssigned(c *gc.C) {
	err := s.st.AssignUnit(defaultInstancePrechecker, s.mysqlUnit, state.AssignNew)
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
	defer state.SetBeforeHooks(c, s.st, func() {
		err := s.mysqlRelunit.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := s.st.AssignUnit(defaultInstancePrechecker, s.mysqlUnit, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)
	_ = s.storageInstanceFilesystem(c, names.NewStorageTag("data/0"))
}
