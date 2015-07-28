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
	"github.com/juju/juju/worker/uniter/runner"
)

type unitStorageSuite struct {
	HookContextSuite
	expectedStorageNames         set.Strings
	charmName                    string
	initCons                     map[string]state.StorageConstraints
	ch                           *state.Charm
	initialStorageInstancesCount int
}

var _ = gc.Suite(&unitStorageSuite{})

const (
	testPool           = "block"
	testPersistentPool = "block-persistent"
)

func (s *unitStorageSuite) SetUpTest(c *gc.C) {
	s.HookContextSuite.SetUpTest(c)
	setupTestStorageSupport(c, s.State)
}

func (s *unitStorageSuite) TestAddUnitStorage(c *gc.C) {
	s.createStorageBlockUnit(c)
	count := uint64(1)
	s.assertUnitStorageAdded(c,
		map[string]params.StorageConstraints{
			"allecto": params.StorageConstraints{Count: &count}})
}

func (s *unitStorageSuite) TestAddUnitStorageIgnoresBlocks(c *gc.C) {
	s.createStorageBlockUnit(c)
	count := uint64(1)
	s.BlockDestroyEnvironment(c, "TestAddUnitStorageIgnoresBlocks")
	s.BlockRemoveObject(c, "TestAddUnitStorageIgnoresBlocks")
	s.BlockAllChanges(c, "TestAddUnitStorageIgnoresBlocks")
	s.assertUnitStorageAdded(c,
		map[string]params.StorageConstraints{
			"allecto": params.StorageConstraints{Count: &count}})
}

func (s *unitStorageSuite) TestAddUnitStorageZeroCount(c *gc.C) {
	s.createStorageBlockUnit(c)
	cons := map[string]params.StorageConstraints{
		"allecto": params.StorageConstraints{}}

	ctx := s.addUnitStorage(c, cons)

	// Flush the context with a success.
	err := ctx.Flush("success", nil)
	c.Assert(err, gc.ErrorMatches, `.*count must be specified.*`)

	// Make sure no storage instances was added
	after, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(after)-s.initialStorageInstancesCount, gc.Equals, 0)
	s.assertExistingStorage(c, after)
}

func (s *unitStorageSuite) TestAddUnitStorageWithSize(c *gc.C) {
	s.createStorageBlockUnit(c)
	size := uint64(1)
	cons := map[string]params.StorageConstraints{
		"allecto": params.StorageConstraints{Size: &size}}

	ctx := s.addUnitStorage(c, cons)

	// Flush the context with a success.
	err := ctx.Flush("success", nil)
	c.Assert(err, gc.ErrorMatches, `.*only count can be specified.*`)

	// Make sure no storage instances was added
	after, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(after)-s.initialStorageInstancesCount, gc.Equals, 0)
	s.assertExistingStorage(c, after)
}

func (s *unitStorageSuite) TestAddUnitStorageWithPool(c *gc.C) {
	s.createStorageBlockUnit(c)
	cons := map[string]params.StorageConstraints{
		"allecto": params.StorageConstraints{Pool: "loop"}}

	ctx := s.addUnitStorage(c, cons)

	// Flush the context with a success.
	err := ctx.Flush("success", nil)
	c.Assert(err, gc.ErrorMatches, `.*only count can be specified.*`)

	// Make sure no storage instances was added
	after, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(after)-s.initialStorageInstancesCount, gc.Equals, 0)
	s.assertExistingStorage(c, after)
}

func (s *unitStorageSuite) TestAddUnitStorageAccumulated(c *gc.C) {
	s.createStorageBlock2Unit(c)
	count := uint64(1)
	s.assertUnitStorageAdded(c,
		map[string]params.StorageConstraints{
			"multi2up": params.StorageConstraints{Count: &count}},
		map[string]params.StorageConstraints{
			"multi1to10": params.StorageConstraints{Count: &count}})
}

func (s *unitStorageSuite) TestAddUnitStorageAccumulatedSame(c *gc.C) {
	s.createStorageBlock2Unit(c)
	count := uint64(1)
	s.assertUnitStorageAdded(c,
		map[string]params.StorageConstraints{
			"multi2up": params.StorageConstraints{Count: &count}},
		map[string]params.StorageConstraints{
			"multi2up": params.StorageConstraints{Count: &count}})
}

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

func (s *unitStorageSuite) createStorageEnabledUnit(c *gc.C) {
	s.ch = s.AddTestingCharm(c, s.charmName)
	s.service = s.AddTestingServiceWithStorage(c, s.charmName, s.ch, s.initCons)
	s.unit = s.AddUnit(c, s.service)

	s.assertStorageCreated(c)
	s.createHookSupport(c)
}

func (s *unitStorageSuite) createStorageBlockUnit(c *gc.C) {
	s.charmName = "storage-block"
	s.initCons = map[string]state.StorageConstraints{
		"data": makeStorageCons("block", 1024, 1),
	}
	s.createStorageEnabledUnit(c)
	s.assertStorageCreated(c)
	s.createHookSupport(c)
}

func (s *unitStorageSuite) createStorageBlock2Unit(c *gc.C) {
	s.charmName = "storage-block2"
	s.initCons = map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("loop", 0, 3),
	}
	s.createStorageEnabledUnit(c)
	s.assertStorageCreated(c)
	s.createHookSupport(c)
}

func (s *unitStorageSuite) assertStorageCreated(c *gc.C) {
	all, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	s.initialStorageInstancesCount = len(all)
	s.expectedStorageNames = set.NewStrings()
	for _, one := range all {
		s.expectedStorageNames.Add(one.StorageName())
	}
}

func (s *unitStorageSuite) createHookSupport(c *gc.C) {
	password, err := utils.RandomPassword()
	err = s.unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAs(c, s.unit.Tag(), password)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.uniter, gc.NotNil)
	s.apiUnit, err = s.uniter.Unit(s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.SetCharmURL(s.ch.URL())
	c.Assert(err, jc.ErrorIsNil)
}

func makeStorageCons(pool string, size, count uint64) state.StorageConstraints {
	return state.StorageConstraints{Pool: pool, Size: size, Count: count}
}

func (s *unitStorageSuite) addUnitStorage(c *gc.C, cons ...map[string]params.StorageConstraints) *runner.HookContext {
	// Get the context.
	ctx := s.getHookContext(c, s.State.EnvironUUID(), -1, "", noProxies)
	c.Assert(ctx.UnitName(), gc.Equals, s.unit.Name())

	for _, one := range cons {
		for storage, _ := range one {
			s.expectedStorageNames.Add(storage)
		}
		ctx.AddUnitStorage(one)
	}
	return ctx
}

func (s *unitStorageSuite) assertUnitStorageAdded(c *gc.C, cons ...map[string]params.StorageConstraints) {
	ctx := s.addUnitStorage(c, cons...)

	// Flush the context with a success.
	err := ctx.Flush("success", nil)
	c.Assert(err, jc.ErrorIsNil)

	after, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(after)-s.initialStorageInstancesCount, gc.Equals, len(cons))
	s.assertExistingStorage(c, after)
}

func (s *unitStorageSuite) assertExistingStorage(c *gc.C, all []state.StorageInstance) {
	for _, one := range all {
		c.Assert(s.expectedStorageNames.Contains(one.StorageName()), jc.IsTrue)
	}
}
