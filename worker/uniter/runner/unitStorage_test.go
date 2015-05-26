// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
)

type unitStorageSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&unitStorageSuite{})

const (
	testPool           = "block"
	testPersistentPool = "block-persistent"
)

func setupTestStorageSupport(c *gc.C, s *state.State) {
	stsetts := state.NewStateSettings(s)
	poolManager := poolmanager.New(stsetts)
	_, err := poolManager.Create(testPool, provider.LoopProviderType, map[string]interface{}{"it": "works"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = poolManager.Create(testPersistentPool, ec2.EBS_ProviderType, map[string]interface{}{"persistent": true})
	c.Assert(err, jc.ErrorIsNil)

	registry.RegisterEnvironStorageProviders("dummy", ec2.EBS_ProviderType)
	registry.RegisterEnvironStorageProviders("dummyenv", ec2.EBS_ProviderType)
}

func (s *unitStorageSuite) SetUpTest(c *gc.C) {
	s.HookContextSuite.SetUpTest(c)
	setupTestStorageSupport(c, s.State)

	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("block", 1024, 1),
	}
	s.service = s.AddTestingServiceWithStorage(c, "storage-block", ch, storage)
	s.unit = s.AddUnit(c, s.service)

	password, err := utils.RandomPassword()
	err = s.unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAs(c, s.unit.Tag(), password)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.uniter, gc.NotNil)
	s.apiUnit, err = s.uniter.Unit(s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.SetCharmURL(ch.URL())
	c.Assert(err, jc.ErrorIsNil)
}

func makeStorageCons(pool string, size, count uint64) state.StorageConstraints {
	return state.StorageConstraints{Pool: pool, Size: size, Count: count}
}

func (s *unitStorageSuite) assertUnitStorageAdded(c *gc.C, cons ...params.StorageConstraints) {
	before, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(before, gc.HasLen, 1)
	c.Assert(before[0].StorageName(), gc.DeepEquals, "data")
	// Get the context.
	ctx := s.getHookContext(c, s.State.EnvironUUID(), -1, "", noProxies)
	c.Assert(ctx.UnitName(), gc.Equals, "storage-block/0")

	for _, one := range cons {
		ctx.AddUnitStorage(map[string]params.StorageConstraints{"allecto": one})
	}

	// Flush the context with a success.
	err = ctx.FlushContext("success", nil)
	c.Assert(err, jc.ErrorIsNil)

	after, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(after)-len(before), gc.Equals, 1)

	expected := set.NewStrings("data", "allecto")
	for _, one := range after {
		c.Assert(expected.Contains(one.StorageName()), jc.IsTrue)
	}
}

func (s *unitStorageSuite) TestAddUnitStorage(c *gc.C) {
	count := uint64(1)
	s.assertUnitStorageAdded(c, params.StorageConstraints{Count: &count})
}

func (s *unitStorageSuite) TestAddUnitStorageZeroCount(c *gc.C) {
	size := uint64(1)
	s.assertUnitStorageAdded(c, params.StorageConstraints{Size: &size})
}

func (s *unitStorageSuite) TestAddUnitStorageAccumulated(c *gc.C) {
	n := uint64(1)
	s.assertUnitStorageAdded(c,
		params.StorageConstraints{Size: &n},
		params.StorageConstraints{Count: &n})
}
