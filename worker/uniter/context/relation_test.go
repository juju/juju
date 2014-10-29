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

	cache := context.NewRelationCache(s.apiRelUnit.ReadSettings, []string{"u/1"})
	ctx := context.NewContextRelation(s.apiRelUnit, cache)

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

	cache := context.NewRelationCache(s.apiRelUnit.ReadSettings, nil)
	ctx := context.NewContextRelation(s.apiRelUnit, cache)

	// Check that settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	expectMap := settings.Map()
	expectSettings := convertMap(expectMap)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that changes to state do not affect the obtained settings.
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, expectSettings)
}

func (s *ContextRelationSuite) TestLocalSettings(c *gc.C) {
	ctx := context.NewContextRelation(s.apiRelUnit, nil)

	// Change Settings...
	node, err := ctx.Settings()
	c.Assert(err, gc.IsNil)
	expectSettings := node.Map()
	expectOldMap := convertSettings(expectSettings)
	node.Set("change", "exciting")

	// ...and check it's not written to state.
	settings, err := s.ru.ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, expectOldMap)

	// Write settings...
	err = ctx.WriteSettings()
	c.Assert(err, gc.IsNil)

	// ...and check it was written to state.
	settings, err = s.ru.ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, map[string]interface{}{"change": "exciting"})
}

func convertSettings(settings params.RelationSettings) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range settings {
		result[k] = v
	}
	return result
}

func convertMap(settingsMap map[string]interface{}) params.RelationSettings {
	result := make(params.RelationSettings)
	for k, v := range settingsMap {
		result[k] = v.(string)
	}
	return result
}
