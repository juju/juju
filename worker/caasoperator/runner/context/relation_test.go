// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator/runner/context"
	"github.com/juju/juju/worker/caasoperator/runner/runnertesting"
)

type ContextRelationSuite struct {
	testing.BaseSuite

	relationAPI  *runnertesting.MockRelationUnitAPI
	relIdCounter int
}

var _ = gc.Suite(&ContextRelationSuite{})

func (s *ContextRelationSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.relationAPI = runnertesting.NewMockRelationUnitAPI(0, "db", true)
}

func (s *ContextRelationSuite) TestMemberCaching(c *gc.C) {
	cache := context.NewRelationCache(s.relationAPI.RemoteSettings, []string{"u/1"})
	ctx := context.NewContextRelation(s.relationAPI, cache)

	settings := runnertesting.Settings{}
	// Check that uncached settings are read from state.
	m, err := ctx.RemoteSettings("u/1")
	c.Assert(err, jc.ErrorIsNil)
	expectMap := settings.Map()
	c.Assert(m.Map(), gc.DeepEquals, expectMap)

	// Check that changes to state do not affect the cached settings.
	settings.Set("ping", "pow")
	err = s.relationAPI.WriteSettings(settings)
	c.Assert(err, jc.ErrorIsNil)

	m, err = ctx.RemoteSettings("u/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Map(), gc.DeepEquals, expectMap)
}

func (s *ContextRelationSuite) TestNonMemberCaching(c *gc.C) {
	cache := context.NewRelationCache(s.relationAPI.RemoteSettings, nil)
	ctx := context.NewContextRelation(s.relationAPI, cache)

	settings := runnertesting.Settings{}
	// Check that settings are read from state.
	m, err := ctx.RemoteSettings("u/1")
	c.Assert(err, jc.ErrorIsNil)
	expectMap := settings.Map()
	c.Assert(m.Map(), gc.DeepEquals, expectMap)

	// Check that changes to state do not affect the obtained settings.
	settings.Set("ping", "pow")
	err = s.relationAPI.WriteSettings(settings)
	c.Assert(err, jc.ErrorIsNil)

	m, err = ctx.RemoteSettings("u/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Map(), gc.DeepEquals, expectMap)
}

func (s *ContextRelationSuite) TestLocalSettings(c *gc.C) {
	ctx := context.NewContextRelation(s.relationAPI, nil)

	// Change Settings...
	node, err := ctx.LocalSettings()
	c.Assert(err, jc.ErrorIsNil)
	expectOldMap := node.Map()
	node.Set("change", "exciting")

	// ...and check it's not written to state.
	settings, err := s.relationAPI.RemoteSettings("u/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings.Map(), gc.DeepEquals, expectOldMap)

	// Write settings...
	err = ctx.WriteSettings()
	c.Assert(err, jc.ErrorIsNil)

	// ...and check it was written to state.
	settings, err = s.relationAPI.RemoteSettings("u/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings.Map(), gc.DeepEquals, map[string]string{"change": "exciting"})
}

func convertSettings(settings map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range settings {
		result[k] = v
	}
	return result
}

func (s *ContextRelationSuite) TestSuspended(c *gc.C) {
	ctx := context.NewContextRelation(s.relationAPI, nil)
	c.Assert(ctx.Suspended(), jc.IsTrue)
}

func (s *ContextRelationSuite) TestSetStatus(c *gc.C) {
	ctx := context.NewContextRelation(s.relationAPI, nil)
	err := ctx.SetStatus(relation.Suspended)
	c.Assert(err, jc.ErrorIsNil)
	relStatus := s.relationAPI.Status()
	c.Assert(relStatus, gc.Equals, relation.Suspended)
}
