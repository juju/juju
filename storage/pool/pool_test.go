// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pool_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/pool"
)

type poolSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&poolSuite{})

var poolAttrs = map[string]interface{}{
	"name": "testpool", "type": "loop", "foo": "bar",
}

func (s *poolSuite) createSettings(c *gc.C) {
	_, err := state.CreateSettings(s.State, "pool#testpool", poolAttrs)
	c.Assert(err, jc.ErrorIsNil)
	// Create settings that isn't a pool.
	_, err = state.CreateSettings(s.State, "r#1", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *poolSuite) TestList(c *gc.C) {
	s.createSettings(c)
	pm := pool.NewPoolManager(s.State)
	pools, err := pm.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
	c.Assert(pools[0].Config(), gc.DeepEquals, poolAttrs)
	c.Assert(pools[0].Name(), gc.Equals, "testpool")
	c.Assert(pools[0].Type(), gc.Equals, storage.ProviderType("loop"))
}

func (s *poolSuite) TestPool(c *gc.C) {
	s.createSettings(c)
	pm := pool.NewPoolManager(s.State)
	p, err := pm.Pool("testpool")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Config(), gc.DeepEquals, poolAttrs)
	c.Assert(p.Name(), gc.Equals, "testpool")
	c.Assert(p.Type(), gc.Equals, storage.ProviderType("loop"))
}

func (s *poolSuite) TestCreate(c *gc.C) {
	pm := pool.NewPoolManager(s.State)
	created, err := pm.Create("testpool", storage.ProviderType("loop"), map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	p, err := pm.Pool("testpool")
	c.Assert(created, gc.DeepEquals, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Config(), gc.DeepEquals, poolAttrs)
	c.Assert(p.Name(), gc.Equals, "testpool")
	c.Assert(p.Type(), gc.Equals, storage.ProviderType("loop"))
}

func (s *poolSuite) TestCreateMissingName(c *gc.C) {
	pm := pool.NewPoolManager(s.State)
	_, err := pm.Create("", "loop", map[string]interface{}{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, "pool name is missing")
}

func (s *poolSuite) TestCreateMissingType(c *gc.C) {
	pm := pool.NewPoolManager(s.State)
	_, err := pm.Create("testpool", "", map[string]interface{}{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, "provider type is missing")
}

func (s *poolSuite) TestDelete(c *gc.C) {
	s.createSettings(c)
	pm := pool.NewPoolManager(s.State)
	err := pm.Delete("testpool")
	c.Assert(err, jc.ErrorIsNil)
	_, err = pm.Pool("testpool")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	// Delete again, no error.
	err = pm.Delete("testpool")
	c.Assert(err, jc.ErrorIsNil)
}
