// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"strings"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/uniter/context"
)

var noProxies = proxy.Settings{}
var apiAddrs = []string{"a1:123", "a2:123"}
var expectedApiAddrs = strings.Join(apiAddrs, " ")

type HookContextSuite struct {
	testing.JujuConnSuite
	service  *state.Service
	unit     *state.Unit
	machine  *state.Machine
	relch    *state.Charm
	relunits map[int]*state.RelationUnit

	st          *api.State
	uniter      *uniter.State
	apiUnit     *uniter.Unit
	apiRelunits map[int]*uniter.RelationUnit
}

func (s *HookContextSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	sch := s.AddTestingCharm(c, "wordpress")
	s.service = s.AddTestingService(c, "u", sch)
	s.unit = s.AddUnit(c, s.service)

	password, err := utils.RandomPassword()
	err = s.unit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAs(c, s.unit.Tag(), password)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, gc.IsNil)
	c.Assert(s.uniter, gc.NotNil)
	s.apiUnit, err = s.uniter.Unit(s.unit.Tag().(names.UnitTag))
	c.Assert(err, gc.IsNil)

	// Note: The unit must always have a charm URL set, because this
	// happens as part of the installation process (that happens
	// before the initial install hook).
	err = s.unit.SetCharmURL(sch.URL())
	c.Assert(err, gc.IsNil)
	s.relch = s.AddTestingCharm(c, "mysql")
	s.relunits = map[int]*state.RelationUnit{}
	s.apiRelunits = map[int]*uniter.RelationUnit{}
	s.AddContextRelation(c, "db0")
	s.AddContextRelation(c, "db1")
}

func (s *HookContextSuite) AddUnit(c *gc.C, svc *state.Service) *state.Unit {
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)
	name := strings.Replace(unit.Name(), "/", "-", 1)
	privateAddr := network.NewAddress(name+".testing.invalid", network.ScopeCloudLocal)
	err = s.machine.SetAddresses(privateAddr)
	c.Assert(err, gc.IsNil)
	return unit
}

func (s *HookContextSuite) AddContextRelation(c *gc.C, name string) {
	s.AddTestingService(c, name, s.relch)
	eps, err := s.State.InferEndpoints("u", name)
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(map[string]interface{}{"relation-name": name})
	c.Assert(err, gc.IsNil)
	s.relunits[rel.Id()] = ru
	apiRel, err := s.uniter.Relation(rel.Tag().(names.RelationTag))
	c.Assert(err, gc.IsNil)
	apiRelUnit, err := apiRel.Unit(s.apiUnit)
	c.Assert(err, gc.IsNil)
	s.apiRelunits[rel.Id()] = apiRelUnit
}

func (s *HookContextSuite) getHookContext(c *gc.C, uuid string, relid int,
	remote string, proxies proxy.Settings, addMetrics bool) *context.HookContext {
	if relid != -1 {
		_, found := s.apiRelunits[relid]
		c.Assert(found, jc.IsTrue)
	}
	facade, err := s.st.Uniter()
	c.Assert(err, gc.IsNil)

	relctxs := map[int]*context.ContextRelation{}
	for relId, relUnit := range s.apiRelunits {
		cache := context.NewRelationCache(relUnit.ReadSettings, nil)
		relctxs[relId] = context.NewContextRelation(relUnit, cache)
	}

	context, err := context.NewHookContext(s.apiUnit, facade, "TestCtx", uuid,
		"test-env-name", relid, remote, relctxs, apiAddrs, names.NewUserTag("owner"),
		proxies, addMetrics, nil, s.machine.Tag().(names.MachineTag))
	c.Assert(err, gc.IsNil)
	return context
}
