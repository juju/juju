// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/uniter/runner"
)

type ContextRelationSuite struct {
	testing.JujuConnSuite
	svc *state.Service
	rel *state.Relation
	ru  *state.RelationUnit

	st         api.Connection
	uniter     *apiuniter.State
	apiRelUnit *apiuniter.RelationUnit
}

var _ = gc.Suite(&ContextRelationSuite{})

func (s *ContextRelationSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	ch := s.AddTestingCharm(c, "riak")
	s.svc = s.AddTestingService(c, "u", ch)
	rels, err := s.svc.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)
	s.rel = rels[0]
	unit, err := s.svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	s.ru, err = s.rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = s.ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	password, err = utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.uniter, gc.NotNil)

	apiRel, err := s.uniter.Relation(s.rel.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	apiUnit, err := s.uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	s.apiRelUnit, err = apiRel.Unit(apiUnit)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ContextRelationSuite) TestMemberCaching(c *gc.C) {
	unit, err := s.svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(map[string]interface{}{"blib": "blob"})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := ru.Settings()
	c.Assert(err, jc.ErrorIsNil)
	settings.Set("ping", "pong")
	_, err = settings.Write()
	c.Assert(err, jc.ErrorIsNil)

	cache := runner.NewRelationCache(s.apiRelUnit.ReadSettings, []string{"u/1"})
	ctx := runner.NewContextRelation(s.apiRelUnit, cache)

	// Check that uncached settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, jc.ErrorIsNil)
	expectMap := settings.Map()
	expectSettings := convertMap(expectMap)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that changes to state do not affect the cached settings.
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, jc.ErrorIsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.DeepEquals, expectSettings)
}

func (s *ContextRelationSuite) TestNonMemberCaching(c *gc.C) {
	unit, err := s.svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(map[string]interface{}{"blib": "blob"})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := ru.Settings()
	c.Assert(err, jc.ErrorIsNil)
	settings.Set("ping", "pong")
	_, err = settings.Write()
	c.Assert(err, jc.ErrorIsNil)

	cache := runner.NewRelationCache(s.apiRelUnit.ReadSettings, nil)
	ctx := runner.NewContextRelation(s.apiRelUnit, cache)

	// Check that settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, jc.ErrorIsNil)
	expectMap := settings.Map()
	expectSettings := convertMap(expectMap)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that changes to state do not affect the obtained settings.
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, jc.ErrorIsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.DeepEquals, expectSettings)
}

func (s *ContextRelationSuite) TestLocalSettings(c *gc.C) {
	ctx := runner.NewContextRelation(s.apiRelUnit, nil)

	// Change Settings...
	node, err := ctx.Settings()
	c.Assert(err, jc.ErrorIsNil)
	expectSettings := node.Map()
	expectOldMap := convertSettings(expectSettings)
	node.Set("change", "exciting")

	// ...and check it's not written to state.
	settings, err := s.ru.ReadSettings("u/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, expectOldMap)

	// Write settings...
	err = ctx.WriteSettings()
	c.Assert(err, jc.ErrorIsNil)

	// ...and check it was written to state.
	settings, err = s.ru.ReadSettings("u/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, map[string]interface{}{"change": "exciting"})
}

func convertSettings(settings params.Settings) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range settings {
		result[k] = v
	}
	return result
}

func convertMap(settingsMap map[string]interface{}) params.Settings {
	result := make(params.Settings)
	for k, v := range settingsMap {
		result[k] = v.(string)
	}
	return result
}
