// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/names"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/uniter/context"
)

type ContextRelationSuite struct {
	testing.JujuConnSuite
	svc *state.Service
	rel *state.Relation
	ru  *state.RelationUnit

	st         *api.State
	uniter     *apiuniter.State
	apiRelUnit *apiuniter.RelationUnit
}

var _ = gc.Suite(&ContextRelationSuite{})

func (s *ContextRelationSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)

	ch := s.AddTestingCharm(c, "riak")
	s.svc = s.AddTestingService(c, "u", ch)
	rels, err := s.svc.Relations()
	c.Assert(err, gc.IsNil)
	c.Assert(rels, gc.HasLen, 1)
	s.rel = rels[0]
	unit, err := s.svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.AssignToMachine(machine)
	s.ru, err = s.rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = s.ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)

	password, err = utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, gc.IsNil)
	c.Assert(s.uniter, gc.NotNil)

	apiRel, err := s.uniter.Relation(s.rel.Tag().(names.RelationTag))
	c.Assert(err, gc.IsNil)
	apiUnit, err := s.uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, gc.IsNil)
	s.apiRelUnit, err = apiRel.Unit(apiUnit)
	c.Assert(err, gc.IsNil)
}

func (s *ContextRelationSuite) TestChangeMembers(c *gc.C) {
	ctx := context.NewContextRelation(s.apiRelUnit, nil)
	c.Assert(ctx.UnitNames(), gc.HasLen, 0)

	// Check the units and settings after a simple update.
	ctx.UpdateMembers(context.SettingsMap{
		"u/2": {"baz": "2"},
		"u/4": {"qux": "4"},
	})
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/2", "u/4"})
	assertSettings := func(unit string, expect params.RelationSettings) {
		actual, err := ctx.ReadSettings(unit)
		c.Assert(err, gc.IsNil)
		c.Assert(actual, gc.DeepEquals, expect)
	}
	assertSettings("u/2", params.RelationSettings{"baz": "2"})
	assertSettings("u/4", params.RelationSettings{"qux": "4"})

	// Send a second update; check that members are only added, not removed.
	ctx.UpdateMembers(context.SettingsMap{
		"u/1": {"foo": "1"},
		"u/2": {"abc": "2"},
		"u/3": {"bar": "3"},
	})
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1", "u/2", "u/3", "u/4"})

	// Check that all settings remain cached.
	assertSettings("u/1", params.RelationSettings{"foo": "1"})
	assertSettings("u/2", params.RelationSettings{"abc": "2"})
	assertSettings("u/3", params.RelationSettings{"bar": "3"})
	assertSettings("u/4", params.RelationSettings{"qux": "4"})

	// Delete a member, and check that it is no longer a member...
	ctx.DeleteMember("u/2")
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1", "u/3", "u/4"})

	// ...and that its settings are no longer cached.
	_, err := ctx.ReadSettings("u/2")
	c.Assert(err, gc.ErrorMatches, "cannot read settings for unit \"u/2\" in relation \"u:ring\": settings not found")
}

func (s *ContextRelationSuite) TestMemberCaching(c *gc.C) {
	unit, err := s.svc.AddUnit()
	c.Assert(err, gc.IsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(map[string]interface{}{"blib": "blob"})
	c.Assert(err, gc.IsNil)
	settings, err := ru.Settings()
	c.Assert(err, gc.IsNil)
	settings.Set("ping", "pong")
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
	ctx := context.NewContextRelation(s.apiRelUnit, map[string]int64{"u/1": 0})

	// Check that uncached settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	expectMap := settings.Map()
	expectSettings := convertMap(expectMap)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that changes to state do not affect the cached settings.
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that ClearCache spares the members cache.
	ctx.ClearCache()
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that updating the context overwrites the cached settings, and
	// that the contents of state are ignored.
	ctx.UpdateMembers(context.SettingsMap{"u/1": {"entirely": "different"}})
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, params.RelationSettings{"entirely": "different"})
}

func (s *ContextRelationSuite) TestNonMemberCaching(c *gc.C) {
	unit, err := s.svc.AddUnit()
	c.Assert(err, gc.IsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(map[string]interface{}{"blib": "blob"})
	c.Assert(err, gc.IsNil)
	settings, err := ru.Settings()
	c.Assert(err, gc.IsNil)
	settings.Set("ping", "pong")
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
	ctx := context.NewContextRelation(s.apiRelUnit, nil)

	// Check that settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	expectMap := settings.Map()
	expectSettings := convertMap(expectMap)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that changes to state do not affect the obtained settings...
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// ...until the caches are cleared.
	ctx.ClearCache()
	c.Assert(err, gc.IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m["ping"], gc.Equals, "pow")
}

func (s *ContextRelationSuite) TestSettings(c *gc.C) {
	ctx := context.NewContextRelation(s.apiRelUnit, nil)

	// Change Settings, then clear cache without writing.
	node, err := ctx.Settings()
	c.Assert(err, gc.IsNil)
	expectSettings := node.Map()
	expectMap := convertSettings(expectSettings)
	node.Set("change", "exciting")
	ctx.ClearCache()

	// Check that the change is not cached...
	node, err = ctx.Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node.Map(), gc.DeepEquals, expectSettings)

	// ...and not written to state.
	settings, err := s.ru.ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, expectMap)

	// Change again, write settings, and clear caches.
	node.Set("change", "exciting")
	err = ctx.WriteSettings()
	c.Assert(err, gc.IsNil)
	ctx.ClearCache()

	// Check that the change is reflected in Settings...
	expectSettings["change"] = "exciting"
	expectMap["change"] = expectSettings["change"]
	node, err = ctx.Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node.Map(), gc.DeepEquals, expectSettings)

	// ...and was written to state.
	settings, err = s.ru.ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, expectMap)
}
