// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
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

}

func makeStorageCons(pool string, size, count uint64) state.StorageConstraints {
	return state.StorageConstraints{Pool: pool, Size: size, Count: count}
}

func (s *unitStorageSuite) TestAddUnitStorage(c *gc.C) {
	ctx := s.HookContextSuite.GetContext(c, -1, "")
	c.Assert(ctx.UnitName(), gc.Equals, "storage-block/0")

	size := uint64(1)
	ok := ctx.AddUnitStorage(
		map[string]params.StorageConstraints{
			"allecto": params.StorageConstraints{Size: &size},
		})
	c.Assert(ok, jc.ErrorIsNil)
}
