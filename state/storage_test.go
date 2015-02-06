// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/state"
)

type StorageStateSuite struct {
	ConnSuite
}

var _ = gc.Suite(&StorageStateSuite{})

func (s *StorageStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	// This suite is all about storage, so enable the feature by default.
	s.PatchEnvironment(osenv.JujuFeatureFlagEnvKey, feature.Storage)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
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

func (s *StorageStateSuite) TestAddServiceStorageConstraints(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block2")
	addService := func(storage map[string]state.StorageConstraints) (*state.Service, error) {
		return s.State.AddService("storage-block2", "user-test-admin@local", ch, nil, storage)
	}
	assertErr := func(storage map[string]state.StorageConstraints, expect string) {
		_, err := addService(storage)
		c.Assert(err, gc.ErrorMatches, expect)
	}
	assertErr(nil, `.*no constraints specified for store.*`)

	storage := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("", 1024, 1),
		"multi2up":   makeStorageCons("", 1024, 1),
	}
	assertErr(storage, `cannot add service "storage-block2": charm "storage-block2" store "multi2up": 2 instances required, 1 specified`)
	storage["multi2up"] = makeStorageCons("", 1024, 2)
	storage["multi1to10"] = makeStorageCons("", 1024, 11)
	assertErr(storage, `cannot add service "storage-block2": charm "storage-block2" store "multi1to10": at most 10 instances supported, 11 specified`)
	storage["multi1to10"] = makeStorageCons("ebs", 1024, 10)
	assertErr(storage, `cannot add service "storage-block2": storage pools are not implemented`)
	storage["multi1to10"] = makeStorageCons("", 1024, 10)
	_, err := addService(storage)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageStateSuite) TestAddUnit(c *gc.C) {
	// Each unit added to the service will create storage instances
	// to satisfy the service's storage constraints.
	ch := s.AddTestingCharm(c, "storage-block2")
	storage := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("", 1024, 1),
		"multi2up":   makeStorageCons("", 1024, 2),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block2", ch, storage)
	for i := 0; i < 2; i++ {
		u, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		storageAttachments, err := s.State.StorageAttachments(u.UnitTag())
		c.Assert(err, jc.ErrorIsNil)
		count := make(map[string]int)
		for _, att := range storageAttachments {
			c.Assert(att.Unit(), gc.Equals, u.UnitTag())
			storageInstance, err := s.State.StorageInstance(att.StorageInstance())
			c.Assert(err, jc.ErrorIsNil)
			count[storageInstance.StorageName()]++
			c.Assert(storageInstance.Kind(), gc.Equals, state.StorageKindBlock)
			_, err = storageInstance.Info()
			c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
		}
		c.Assert(count, gc.DeepEquals, map[string]int{
			"multi1to10": 1,
			"multi2up":   2,
		})
	}
}

// TODO(axw) StorageInstance can't be destroyed while it has attachments
// TODO(axw) StorageAttachments can't be added to Dying StorageInstance
// TODO(axw) StorageInstance becomes Dying when Unit becomes Dying
// TODO(axw) StorageAttachments become Dying when StorageInstance becomes Dying
