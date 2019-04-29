// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&FirewallSuite{})

type FirewallSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
}

func (s *FirewallSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState(coretesting.ModelTag.Id())
}

func (s *FirewallSuite) TestWatchEgressAddressesForRelations(c *gc.C) {
	db2Relation := newMockRelation(123)
	db2Relation.ruwApp = "django"
	// Initial event.
	db2Relation.ew.changes <- []string{}
	db2Relation.endpoints = []state.Endpoint{
		{
			ApplicationName: "django",
			Relation: charm.Relation{
				Name:      "db",
				Interface: "db2",
				Role:      "requirer",
				Limit:     1,
				Scope:     charm.ScopeGlobal,
			},
		},
	}
	s.st.relations["remote-db2:db django:db"] = db2Relation
	s.st.remoteEntities[names.NewRelationTag("remote-db2:db django:db")] = "token-db2:db django:db"
	// django/0 and django/1 are initially in scope
	db2Relation.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/0": {},
			"django/1": {},
		},
	}

	unit := newMockUnit("django/0")
	unit.publicAddress = network.NewScopedAddress("1.2.3.4", network.ScopePublic)
	unit.machineId = "0"
	s.st.units["django/0"] = unit
	unit1 := newMockUnit("django/1")
	unit1.publicAddress = network.NewScopedAddress("4.3.2.1", network.ScopePublic)
	unit1.machineId = "1"
	s.st.units["django/1"] = unit1
	s.st.machines["0"] = newMockMachine("0")
	s.st.machines["1"] = newMockMachine("1")
	app := newMockApplication("django")
	app.units = []*mockUnit{unit, unit1}
	s.st.applications["django"] = app

	result, err := firewall.WatchEgressAddressesForRelations(
		s.resources, s.st,
		params.Entities{Entities: []params.Entity{{
			Tag: names.NewRelationTag("remote-db2:db django:db").String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Changes, jc.SameContents, []string{"1.2.3.4/32", "4.3.2.1/32"})
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))

	s.st.CheckCallNames(c, "KeyRelation", "Application", "Unit", "Machine", "Unit", "Machine")
	s.st.CheckCall(c, 0, "KeyRelation", "remote-db2:db django:db")
	s.st.CheckCall(c, 1, "Application", "django")

	django0Call := s.st.Calls()[2]
	django0MachineCall := s.st.Calls()[3]
	django1Call := s.st.Calls()[4]
	django1MachineCall := s.st.Calls()[5]

	c.Assert(django0Call.Args, gc.HasLen, 1)
	if django0Call.Args[0] == "django/1" {
		django0Call, django1Call = django1Call, django0Call
		django0MachineCall, django1MachineCall = django1MachineCall, django0MachineCall
	}
	c.Assert(django0Call.Args, jc.DeepEquals, []interface{}{"django/0"})
	c.Assert(django0MachineCall.Args, jc.DeepEquals, []interface{}{"0"})
	c.Assert(django1Call.Args, jc.DeepEquals, []interface{}{"django/1"})
	c.Assert(django1MachineCall.Args, jc.DeepEquals, []interface{}{"1"})
}

func (s *FirewallSuite) TestWatchEgressAddressesForRelationsIgnoresProvider(c *gc.C) {
	db2Relation := newMockRelation(123)
	// Initial event.
	db2Relation.ew.changes <- []string{}
	db2Relation.endpoints = []state.Endpoint{
		{
			ApplicationName: "db2",
			Relation: charm.Relation{
				Name:      "data",
				Interface: "db2",
				Role:      "provider",
				Limit:     1,
				Scope:     charm.ScopeGlobal,
			},
		},
	}

	s.st.relations["remote-db2:db django:db"] = db2Relation
	app := newMockApplication("db2")
	s.st.applications["db2"] = app
	s.st.remoteEntities[names.NewRelationTag("remote-db2:db django:db")] = "token-db2:db django:db"

	result, err := firewall.WatchEgressAddressesForRelations(
		s.resources, s.st,
		params.Entities{Entities: []params.Entity{{
			Tag: names.NewRelationTag("remote-db2:db django:db").String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "egress network for application db2 without requires endpoint not supported")
}
