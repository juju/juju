// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package poolmanager_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

type poolSuite struct {
	// TODO - don't use state directly, mock it out and add feature tests.
	statetesting.StateSuite
	poolManager poolmanager.PoolManager
	settings    poolmanager.SettingsManager
}

var _ = gc.Suite(&poolSuite{})

var poolAttrs = map[string]interface{}{
	"name": "testpool", "type": "loop", "foo": "bar",
}

func (s *poolSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.settings = state.NewStateSettings(s.State)
	s.poolManager = poolmanager.New(s.settings)
}

func (s *poolSuite) createSettings(c *gc.C) {
	err := s.settings.CreateSettings("pool#testpool", poolAttrs)
	c.Assert(err, jc.ErrorIsNil)
	// Create settings that isn't a pool.
	err = s.settings.CreateSettings("r#1", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *poolSuite) TestList(c *gc.C) {
	s.createSettings(c)
	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
	c.Assert(pools[0].Attrs(), gc.DeepEquals, map[string]interface{}{"foo": "bar"})
	c.Assert(pools[0].Name(), gc.Equals, "testpool")
	c.Assert(pools[0].Provider(), gc.Equals, storage.ProviderType("loop"))
}

func (s *poolSuite) TestListManyResults(c *gc.C) {
	s.createSettings(c)
	err := s.settings.CreateSettings("pool#testpool2", map[string]interface{}{
		"name": "testpool2", "type": "loop", "foo2": "bar2",
	})
	c.Assert(err, jc.ErrorIsNil)
	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 2)
	poolCfgs := make(map[string]map[string]interface{})
	for _, p := range pools {
		poolCfgs[p.Name()] = p.Attrs()
	}
	c.Assert(poolCfgs, jc.DeepEquals, map[string]map[string]interface{}{
		"testpool":  {"foo": "bar"},
		"testpool2": {"foo2": "bar2"},
	})
}

func (s *poolSuite) TestListNoPools(c *gc.C) {
	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 0)
}

func (s *poolSuite) TestPool(c *gc.C) {
	s.createSettings(c)
	p, err := s.poolManager.Get("testpool")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Attrs(), gc.DeepEquals, map[string]interface{}{"foo": "bar"})
	c.Assert(p.Name(), gc.Equals, "testpool")
	c.Assert(p.Provider(), gc.Equals, storage.ProviderType("loop"))
}

func (s *poolSuite) TestCreate(c *gc.C) {
	created, err := s.poolManager.Create("testpool", storage.ProviderType("loop"), map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	p, err := s.poolManager.Get("testpool")
	c.Assert(created, gc.DeepEquals, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Attrs(), gc.DeepEquals, map[string]interface{}{"foo": "bar"})
	c.Assert(p.Name(), gc.Equals, "testpool")
	c.Assert(p.Provider(), gc.Equals, storage.ProviderType("loop"))
}

func (s *poolSuite) TestCreateAlreadyExists(c *gc.C) {
	_, err := s.poolManager.Create("testpool", storage.ProviderType("loop"), map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.poolManager.Create("testpool", storage.ProviderType("loop"), map[string]interface{}{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, ".*cannot overwrite.*")
}

func (s *poolSuite) TestCreateMissingName(c *gc.C) {
	_, err := s.poolManager.Create("", "loop", map[string]interface{}{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, "pool name is missing")
}

func (s *poolSuite) TestCreateMissingType(c *gc.C) {
	_, err := s.poolManager.Create("testpool", "", map[string]interface{}{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, "provider type is missing")
}

func (s *poolSuite) TestCreateInvalidConfig(c *gc.C) {
	_, err := s.poolManager.Create("testpool", storage.ProviderType("loop"), map[string]interface{}{"persistent": true})
	c.Assert(err, gc.ErrorMatches, `validating storage provider config: machine scoped storage provider "testpool" does not support persistent storage`)
}

func (s *poolSuite) TestDelete(c *gc.C) {
	s.createSettings(c)
	err := s.poolManager.Delete("testpool")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.poolManager.Get("testpool")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	// Delete again, no error.
	err = s.poolManager.Delete("testpool")
	c.Assert(err, jc.ErrorIsNil)
}
