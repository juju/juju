// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
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
	})
	registry.RegisterProvider("machinescoped", &dummy.StorageProvider{
		StorageScope: storage.ScopeMachine,
	})
	registry.RegisterEnvironStorageProviders(
		"someprovider", "environscoped", "machinescoped",
	)
	s.AddSuiteCleanup(func(c *gc.C) {
		registry.RegisterProvider("environscoped", nil)
		registry.RegisterProvider("machinescoped", nil)
	})
}

func (s *StorageStateSuiteBase) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	// This suite is all about storage, so enable the feature by default.
	s.PatchEnvironment(osenv.JujuFeatureFlagEnvKey, feature.Storage)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)

	// Create a default pool for block devices.
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType)
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

func makeStorageCons(pool string, size, count uint64) state.StorageConstraints {
	return state.StorageConstraints{Pool: pool, Size: size, Count: count}
}

func (s *StorageStateSuite) TestAddServiceStorageConstraintsWithoutFeature(c *gc.C) {
	// Disable the storage feature, and ensure we can deploy a service from
	// a charm that defines storage, without specifying the storage constraints.
	s.PatchEnvironment(osenv.JujuFeatureFlagEnvKey, "")
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)

	ch := s.AddTestingCharm(c, "storage-block2")
	service, err := s.State.AddService("storage-block2", "user-test-admin@local", ch, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	storageConstraints, err := service.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageConstraints, gc.HasLen, 0)
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
	assertErr(nil, `.*no constraints specified for store.*`)

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

func (s *StorageStateSuite) TestAddServiceStorageConstraintsDefaultPool(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 2048, 1),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop-pool", 2048, 1),
	}
	s.assertAddServiceStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddServiceStorageConstraintsNoUserDefaultPool(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 2048, 1),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop", 2048, 1),
	}
	s.assertAddServiceStorageConstraintsDefaults(c, "", storageCons, expectedCons)
}

func (s *StorageStateSuite) TestAddServiceStorageConstraintsDefaultSizeFallback(c *gc.C) {
	storageCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop-pool", 0, 1),
	}
	expectedCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop-pool", 1024, 1),
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
	err = s.State.EnsureStorageAttachmentDead(storageTag, u.UnitTag())
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

	err = s.State.EnsureStorageAttachmentDead(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestConcurrentDestroyStorageInstanceRemoveStorageAttachmentsRemovesInstance(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.EnsureStorageAttachmentDead(storageTag, u.UnitTag())
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

	ensureDead := func() {
		err = s.State.EnsureStorageAttachmentDead(storageTag, u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
	}
	remove := func() {
		err = s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
	}

	defer state.SetBeforeHooks(c, s.State, ensureDead, remove).Check()
	ensureDead()
	remove()
	exists := s.storageInstanceExists(c, storageTag)
	c.Assert(exists, jc.IsFalse)
}

func (s *StorageStateSuite) TestRemoveAliveStorageAttachmentError(c *gc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "block", "loop-pool")

	err := s.State.RemoveStorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, gc.ErrorMatches, "cannot remove storage attachment data/0:storage-block/0: storage attachment is not dead")

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
	err := s.State.EnsureStorageAttachmentDead(storageTag, u.UnitTag())
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

// TODO(axw) StorageAttachments can't be added to Dying StorageInstance
// TODO(axw) StorageInstance without attachments is removed by Destroy
// TODO(axw) StorageInstance becomes Dying when Unit becomes Dying
// TODO(axw) concurrent add-unit and StorageAttachment removal does not
//           remove storage instance.
