// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/remoterelations"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&remoteRelationsSuite{})

type remoteRelationsSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	api        *remoterelations.RemoteRelationsAPI
}

func (s *remoteRelationsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("0"),
		EnvironManager: true,
	}

	s.st = newMockState()
	api, err := remoterelations.NewRemoteRelationsAPI(s.st, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *remoteRelationsSuite) TestWatchRemoteServices(c *gc.C) {
	serviceNames := []string{"db2", "hadoop"}
	s.st.remoteServicesWatcher.changes <- serviceNames
	result, err := s.api.WatchRemoteServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, serviceNames)

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))
}

func (s *remoteRelationsSuite) TestWatchRemoteService(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	db2RelationUnitsWatcher := newMockRelationUnitsWatcher()
	db2RelationUnitsWatcher.changes <- multiwatcher.RelationUnitsChange{
		Changed: map[string]multiwatcher.UnitSettings{"django/0": {}},
	}
	db2Relation := newMockRelation(123)
	db2Relation.units["django/0"] = djangoRelationUnit
	db2Relation.endpointUnitsWatchers["db2"] = db2RelationUnitsWatcher
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.serviceRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteService(params.Entities{[]params.Entity{
		{"service-db2"},
		{"service-hadoop"},
		{"machine-42"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ServiceRelationsWatchResult{{
		ServiceRelationsWatcherId: "1",
		Changes: &params.ServiceRelationsChange{
			ChangedRelations: map[int]params.RelationChange{
				123: {
					Life: params.Alive,
					ChangedUnits: map[string]params.RelationUnitChange{
						"django/0": {
							Settings: djangoRelationUnit.settings,
						},
					},
				},
			},
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: `service "hadoop" not found`,
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid service tag`,
		},
	}})

	s.st.CheckCalls(c, []testing.StubCall{
		{"WatchRemoteServiceRelations", []interface{}{"db2"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"WatchRemoteServiceRelations", []interface{}{"hadoop"}},
	})

	db2Relation.CheckCalls(c, []testing.StubCall{
		{"Id", []interface{}{}},
		{"Life", []interface{}{}},
		{"WatchCounterpartEndpointUnits", []interface{}{"db2"}},
		{"Unit", []interface{}{"django/0"}},
	})
}

func (s *remoteRelationsSuite) TestWatchRemoteServiceRelationRemoved(c *gc.C) {
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	db2RelationUnitsWatcher := newMockRelationUnitsWatcher()
	db2RelationUnitsWatcher.changes <- multiwatcher.RelationUnitsChange{}
	db2Relation := newMockRelation(123)
	db2Relation.endpointUnitsWatchers["db2"] = db2RelationUnitsWatcher
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.serviceRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteService(params.Entities{[]params.Entity{{"service-db2"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ServiceRelationsWatchResult{{
		ServiceRelationsWatcherId: "1",
		Changes: &params.ServiceRelationsChange{
			// The relation is not found, but it was never reported
			// to us, so it should not be reported in "Removed".
			ChangedRelations: map[int]params.RelationChange{
				123: {Life: params.Alive},
			},
		},
	}})

	// Remove the relation, and expect it to be reported as removed.
	delete(s.st.relations, "db2:db django:db")
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	w := s.resources.Get("1").(apiserver.ServiceRelationsWatcher)
	change := <-w.Changes()
	c.Assert(change, jc.DeepEquals, params.ServiceRelationsChange{
		RemovedRelations: []int{123},
	})

	s.st.CheckCalls(c, []testing.StubCall{
		{"WatchRemoteServiceRelations", []interface{}{"db2"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
	db2Relation.CheckCalls(c, []testing.StubCall{
		{"Id", []interface{}{}},
		{"Life", []interface{}{}},
		{"WatchCounterpartEndpointUnits", []interface{}{"db2"}},
	})
	db2RelationUnitsWatcher.CheckCallNames(c, "Changes", "Changes", "Stop")
}

func (s *remoteRelationsSuite) TestWatchRemoteServiceRelationRemovedRace(c *gc.C) {
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	s.st.serviceRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteService(params.Entities{[]params.Entity{{"service-db2"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ServiceRelationsWatchResult{{
		ServiceRelationsWatcherId: "1",
		// The relation is not found, but it was never reported
		// to us, so it should not be reported in "Removed".
		Changes: &params.ServiceRelationsChange{},
	}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"WatchRemoteServiceRelations", []interface{}{"db2"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *remoteRelationsSuite) TestWatchRemoteServiceRelationUnitRemoved(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	db2RelationUnitsWatcher := newMockRelationUnitsWatcher()
	db2RelationUnitsWatcher.changes <- multiwatcher.RelationUnitsChange{
		Changed: map[string]multiwatcher.UnitSettings{"django/0": {}},
	}
	db2Relation := newMockRelation(123)
	db2Relation.units["django/0"] = djangoRelationUnit
	db2Relation.endpointUnitsWatchers["db2"] = db2RelationUnitsWatcher
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.serviceRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteService(params.Entities{[]params.Entity{{"service-db2"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ServiceRelationsWatchResult{{
		ServiceRelationsWatcherId: "1",
		Changes: &params.ServiceRelationsChange{
			ChangedRelations: map[int]params.RelationChange{
				123: {
					Life: params.Alive,
					ChangedUnits: map[string]params.RelationUnitChange{
						"django/0": {
							Settings: djangoRelationUnit.settings,
						},
					},
				},
			},
		},
	}})

	db2RelationUnitsWatcher.changes <- multiwatcher.RelationUnitsChange{
		Departed: []string{"django/0"},
	}
	w := s.resources.Get("1").(apiserver.ServiceRelationsWatcher)
	change := <-w.Changes()
	c.Assert(change, jc.DeepEquals, params.ServiceRelationsChange{
		ChangedRelations: map[int]params.RelationChange{
			123: {
				Life:          params.Alive,
				DepartedUnits: []string{"django/0"},
			},
		},
	})

	db2Relation.CheckCalls(c, []testing.StubCall{
		{"Id", []interface{}{}},
		{"Life", []interface{}{}},
		{"WatchCounterpartEndpointUnits", []interface{}{"db2"}},
		{"Unit", []interface{}{"django/0"}},
	})
}

func (s *remoteRelationsSuite) TestWatchRemoteServiceRelationUnitRemovedRace(c *gc.C) {
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	db2RelationUnitsWatcher := newMockRelationUnitsWatcher()
	db2RelationUnitsWatcher.changes <- multiwatcher.RelationUnitsChange{
		Departed: []string{"django/0"},
	}
	db2Relation := newMockRelation(123)
	db2Relation.endpointUnitsWatchers["db2"] = db2RelationUnitsWatcher
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.serviceRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteService(params.Entities{[]params.Entity{{"service-db2"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ServiceRelationsWatchResult{{
		ServiceRelationsWatcherId: "1",
		Changes: &params.ServiceRelationsChange{
			ChangedRelations: map[int]params.RelationChange{
				123: {Life: params.Alive},
			},
		},
	}})
}
