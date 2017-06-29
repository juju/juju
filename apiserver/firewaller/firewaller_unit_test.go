// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/firewaller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&RemoteFirewallerSuite{})

type RemoteFirewallerSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	api        *firewaller.FirewallerAPIV4
}

func (s *RemoteFirewallerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState(coretesting.ModelTag.Id())
	api, err := firewaller.NewFirewallerAPI(s.st, s.resources, s.authorizer, &mockcloudSpecAPI{})
	c.Assert(err, jc.ErrorIsNil)
	s.api = &firewaller.FirewallerAPIV4{api}
}

func (s *RemoteFirewallerSuite) TestWatchEgressAddressesForRelations(c *gc.C) {
	db2Relation := newMockRelation(123)
	db2Relation.ruwApp = "django"
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
	db2Relation.inScope = set.NewStrings("django/0", "django/1")
	s.st.relations["remote-db2:db django:db"] = db2Relation
	s.st.remoteEntities[names.NewRelationTag("remote-db2:db django:db")] = "token-db2:db django:db"

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

	result, err := s.api.WatchEgressAddressesForRelations(
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

	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"remote-db2:db django:db"}},
		{"Application", []interface{}{"django"}},
		{"Application", []interface{}{"django"}},
		{"Machine", []interface{}{"0"}},
		{"Machine", []interface{}{"1"}},
	})
}

func (s *RemoteFirewallerSuite) TestWatchEgressAddressesForRelationsIgnoresProvider(c *gc.C) {
	db2Relation := newMockRelation(123)
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

	result, err := s.api.WatchEgressAddressesForRelations(
		params.Entities{Entities: []params.Entity{{
			Tag: names.NewRelationTag("remote-db2:db django:db").String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "egress network for application db2 without requires endpoint not supported")
}

func (s *RemoteFirewallerSuite) TestWatchIngressAddressesForRelations(c *gc.C) {
	db2Relation := newMockRelation(123)
	s.st.relations["remote-db2:db django:db"] = db2Relation

	result, err := s.api.WatchIngressAddressesForRelations(
		params.Entities{Entities: []params.Entity{{
			Tag: names.NewRelationTag("remote-db2:db django:db").String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Changes, jc.SameContents, []string{"1.2.3.4/32"})
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))

	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"remote-db2:db django:db"}},
	})
}
