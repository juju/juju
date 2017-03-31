// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/remotefirewaller"
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
	api        *remotefirewaller.FirewallerAPI
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
	api, err := remotefirewaller.NewRemoteFirewallerAPI(s.st, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *RemoteFirewallerSuite) TestWatchIngressAddressesForRelation(c *gc.C) {
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
	s.st.relations["remote-db2:db django:db"] = db2Relation
	app := newMockApplication("django")
	s.st.applications["django"] = app
	s.st.remoteEntities[names.NewRelationTag("remote-db2:db django:db")] = "token-db2:db django:db"

	unit := newMockUnit("django/0")
	unit.publicAddress = network.NewScopedAddress("1.2.3.4", network.ScopePublic)
	s.st.units["django/0"] = unit
	unit1 := newMockUnit("django/0")
	unit1.publicAddress = network.NewScopedAddress("4.3.2.1", network.ScopePublic)
	s.st.units["django/1"] = unit1

	db2Relation.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/0": {},
			"django/1": {},
		},
	}

	result, err := s.api.WatchIngressAddressesForRelation(
		params.RemoteEntities{Entities: []params.RemoteEntityId{{
			ModelUUID: coretesting.ModelTag.Id(), Token: "token-db2:db django:db"}},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Changes, jc.SameContents, []string{"1.2.3.4/32", "4.3.2.1/32"})
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))

	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{names.NewModelTag(coretesting.ModelTag.Id()), "token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"remote-db2:db django:db"}},
		{"Application", []interface{}{"django"}},
		{"Unit", []interface{}{"django/0"}},
		{"Unit", []interface{}{"django/1"}},
	})
}

func (s *RemoteFirewallerSuite) xTestWatchIngressAddressesForRelationIgnoresProvider(c *gc.C) {
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

	result, err := s.api.WatchIngressAddressesForRelation(
		params.RemoteEntities{Entities: []params.RemoteEntityId{{
			ModelUUID: coretesting.ModelTag.Id(), Token: "token-db2:db django:db"}},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "ingress network for application db2 without requires endpoint not supported")
}
