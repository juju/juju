// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/remoterelations"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
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

func (s *remoteRelationsSuite) TestWatchRemoteApplications(c *gc.C) {
	applicationNames := []string{"db2", "hadoop"}
	s.st.remoteApplicationsWatcher.changes <- applicationNames
	result, err := s.api.WatchRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, applicationNames)

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))
}

func (s *remoteRelationsSuite) TestWatchRemoteApplicationRelations(c *gc.C) {
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	s.st.applicationRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteApplicationRelations(params.Entities{[]params.Entity{
		{"application-db2"},
		{"application-hadoop"},
		{"machine-42"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.StringsWatchResult{{
		StringsWatcherId: "1",
		Changes:          []string{"db2:db django:db"},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: `application "hadoop" not found`,
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid application tag`,
		},
	}})

	s.st.CheckCalls(c, []testing.StubCall{
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"WatchRemoteApplicationRelations", []interface{}{"hadoop"}},
	})
}

// TODO(wallyworld) - underlying code not currently used
func (s *remoteRelationsSuite) xTestWatchRemoteApplicationRelations(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	db2RelationUnitsWatcher := newMockRelationUnitsWatcher()
	db2RelationUnitsWatcher.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{"django/0": {}},
	}
	db2Relation := newMockRelation(123)
	db2Relation.units["django/0"] = djangoRelationUnit
	db2Relation.endpointUnitsWatchers["db2"] = db2RelationUnitsWatcher
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.applicationRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteApplicationRelations(params.Entities{[]params.Entity{
		{"application-db2"},
		{"application-hadoop"},
		{"machine-42"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.RemoteRelationsWatchResult{{
		RemoteRelationsWatcherId: "1",
		Change: &params.RemoteRelationsChange{
			ChangedRelations: []params.RemoteRelationChange{{
				RelationId: 123,
				Life:       params.Alive,
				ChangedUnits: map[string]params.RemoteRelationUnitChange{
					"django/0": {
						Settings: djangoRelationUnit.settings,
					},
				},
			}},
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: `application "hadoop" not found`,
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid application tag`,
		},
	}})

	s.st.CheckCalls(c, []testing.StubCall{
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"WatchRemoteApplicationRelations", []interface{}{"hadoop"}},
	})

	db2Relation.CheckCalls(c, []testing.StubCall{
		{"Id", []interface{}{}},
		{"Life", []interface{}{}},
		{"WatchUnits", []interface{}{"db2"}},
		{"Unit", []interface{}{"django/0"}},
	})
}

// TODO(wallyworld) - underlying code not currently used
func (s *remoteRelationsSuite) xTestWatchRemoteApplicationRelationRemoved(c *gc.C) {
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	db2RelationUnitsWatcher := newMockRelationUnitsWatcher()
	db2RelationUnitsWatcher.changes <- params.RelationUnitsChange{}
	db2Relation := newMockRelation(123)
	db2Relation.endpointUnitsWatchers["db2"] = db2RelationUnitsWatcher
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.applicationRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteApplicationRelations(params.Entities{[]params.Entity{{"application-db2"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.RemoteRelationsWatchResult{{
		RemoteRelationsWatcherId: "1",
		Change: &params.RemoteRelationsChange{
			// The relation is not found, but it was never reported
			// to us, so it should not be reported in "Removed".
			ChangedRelations: []params.RemoteRelationChange{{
				RelationId: 123,
				Life:       params.Alive,
			}},
		},
	}})

	// Remove the relation, and expect it to be reported as removed.
	delete(s.st.relations, "db2:db django:db")
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	w := s.resources.Get("1").(apiserver.RemoteRelationsWatcher)
	change := <-w.Changes()
	c.Assert(change, jc.DeepEquals, params.RemoteRelationsChange{
		RemovedRelations: []int{123},
	})

	s.st.CheckCalls(c, []testing.StubCall{
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
	db2Relation.CheckCalls(c, []testing.StubCall{
		{"Id", []interface{}{}},
		{"Life", []interface{}{}},
		{"WatchUnits", []interface{}{"db2"}},
	})
	db2RelationUnitsWatcher.CheckCallNames(c, "Changes", "Changes", "Stop")
}

// TODO(wallyworld) - underlying code not currently used
func (s *remoteRelationsSuite) xTestWatchRemoteApplicationRelationRemovedRace(c *gc.C) {
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	s.st.applicationRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteApplicationRelations(params.Entities{[]params.Entity{{"application-db2"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.RemoteRelationsWatchResult{{
		RemoteRelationsWatcherId: "1",
		// The relation is not found, but it was never reported
		// to us, so it should not be reported in "Removed".
		Change: &params.RemoteRelationsChange{},
	}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

// TODO(wallyworld) - underlying code not currently used
func (s *remoteRelationsSuite) xTestWatchRemoteApplicationRelationUnitRemoved(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	db2RelationUnitsWatcher := newMockRelationUnitsWatcher()
	db2RelationUnitsWatcher.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{"django/0": {}},
	}
	db2Relation := newMockRelation(123)
	db2Relation.units["django/0"] = djangoRelationUnit
	db2Relation.endpointUnitsWatchers["db2"] = db2RelationUnitsWatcher
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.applicationRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteApplicationRelations(params.Entities{[]params.Entity{{"application-db2"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.RemoteRelationsWatchResult{{
		RemoteRelationsWatcherId: "1",
		Change: &params.RemoteRelationsChange{
			ChangedRelations: []params.RemoteRelationChange{{
				RelationId: 123,
				Life:       params.Alive,
				ChangedUnits: map[string]params.RemoteRelationUnitChange{
					"django/0": {
						Settings: djangoRelationUnit.settings,
					},
				},
			}},
		},
	}})

	db2RelationUnitsWatcher.changes <- params.RelationUnitsChange{
		Departed: []string{"django/0"},
	}
	w := s.resources.Get("1").(apiserver.RemoteRelationsWatcher)
	change := <-w.Changes()
	c.Assert(change, jc.DeepEquals, params.RemoteRelationsChange{
		ChangedRelations: []params.RemoteRelationChange{{
			RelationId:    123,
			Life:          params.Alive,
			DepartedUnits: []string{"django/0"},
		}},
	})

	db2Relation.CheckCalls(c, []testing.StubCall{
		{"Id", []interface{}{}},
		{"Life", []interface{}{}},
		{"WatchUnits", []interface{}{"db2"}},
		{"Unit", []interface{}{"django/0"}},
	})
}

// TODO(wallyworld) - underlying code not currently used
func (s *remoteRelationsSuite) xTestWatchRemoteApplicationRelationUnitRemovedRace(c *gc.C) {
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	db2RelationUnitsWatcher := newMockRelationUnitsWatcher()
	db2RelationUnitsWatcher.changes <- params.RelationUnitsChange{
		Departed: []string{"django/0"},
	}
	db2Relation := newMockRelation(123)
	db2Relation.endpointUnitsWatchers["db2"] = db2RelationUnitsWatcher
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.applicationRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteApplicationRelations(params.Entities{[]params.Entity{{"application-db2"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.RemoteRelationsWatchResult{{
		RemoteRelationsWatcherId: "1",
		Change: &params.RemoteRelationsChange{
			ChangedRelations: []params.RemoteRelationChange{{
				RelationId: 123,
				Life:       params.Alive,
			}},
		},
	}})
}

func (s *remoteRelationsSuite) TestPublishLocalRelationsChange(c *gc.C) {
	_, err := s.api.PublishLocalRelationChange(params.RemoteRelationsChanges{})
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *remoteRelationsSuite) TestConsumeRemoveApplicationChange(c *gc.C) {
	mysql := newMockRemoteApplication("mysql", "local:/u/me/mysql")
	s.st.remoteApplications["mysql"] = mysql

	relation1 := newMockRelation(1)
	relation2 := newMockRelation(2)
	relation3 := newMockRelation(3)
	s.st.relations["mysql:db wordpress:db"] = relation1
	s.st.relations["mysql:db django:db"] = relation2
	s.st.relations["mysql:munin munin:munin-node"] = relation3

	mysql0 := newMockRelationUnit()
	mysql1 := newMockRelationUnit()
	relation1.units["mysql/0"] = mysql0
	relation2.units["mysql/1"] = mysql1
	mysql0Settings := map[string]interface{}{"k": "v"}

	results, err := s.api.ConsumeRemoteApplicationChange(params.RemoteApplicationChanges{
		Changes: []params.RemoteApplicationChange{{
			ApplicationTag: "application-mysql",
			Life:           params.Alive,
			Relations: params.RemoteRelationsChange{
				ChangedRelations: []params.RemoteRelationChange{{
					RelationId: 1,
					Life:       params.Alive,
					ChangedUnits: map[string]params.RemoteRelationUnitChange{
						"mysql/0": {Settings: mysql0Settings},
					},
				}, {
					RelationId:    2,
					Life:          params.Dying,
					DepartedUnits: []string{"mysql/1"},
				}},
				RemovedRelations: []int{3, 42},
			},
		}, {
			ApplicationTag: "application-db2",
		}, {
			ApplicationTag: "machine-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{nil},
		{&params.Error{Code: params.CodeNotFound, Message: `remote application "db2" not found`}},
		{&params.Error{Code: "", Message: `"machine-42" is not a valid application tag`}},
	})

	s.st.CheckCalls(c, []testing.StubCall{
		{"RemoteApplication", []interface{}{"mysql"}},
		{"Relation", []interface{}{int(3)}},
		{"Relation", []interface{}{int(42)}},
		{"Relation", []interface{}{int(1)}},
		{"Relation", []interface{}{int(2)}},
		{"RemoteApplication", []interface{}{"db2"}},
	})
	mysql.CheckCalls(c, []testing.StubCall{}) // no calls yet

	relation1.CheckCalls(c, []testing.StubCall{
		{"RemoteUnit", []interface{}{"mysql/0"}},
	})
	relation2.CheckCalls(c, []testing.StubCall{
		{"Destroy", []interface{}{}},
		{"RemoteUnit", []interface{}{"mysql/1"}},
	})
	relation3.CheckCalls(c, []testing.StubCall{
		{"Destroy", []interface{}{}},
	})

	mysql0.CheckCalls(c, []testing.StubCall{
		{"InScope", []interface{}{}},
		{"EnterScope", []interface{}{mysql0Settings}},
	})
	mysql1.CheckCalls(c, []testing.StubCall{
		{"LeaveScope", []interface{}{}},
	})
}

func (s *remoteRelationsSuite) TestWatchLocalRelationUnits(c *gc.C) {
	djangoRelationUnitsWatcher := newMockRelationUnitsWatcher()
	djangoRelationUnitsWatcher.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{"django/0": {Version: 1}},
	}
	djangoRelation := newMockRelation(123)
	djangoRelation.endpointUnitsWatchers["django"] = djangoRelationUnitsWatcher
	djangoRelation.endpoints = []state.Endpoint{{
		ApplicationName: "db2",
	}, {
		ApplicationName: "django",
	}}

	s.st.relations["django:db db2:db"] = djangoRelation
	s.st.applications["django"] = newMockApplication("django")

	results, err := s.api.WatchLocalRelationUnits(params.Entities{[]params.Entity{
		{"relation-django:db#db2:db"},
		{"relation-hadoop:db#db2:db"},
		{"machine-42"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.RelationUnitsWatchResult{{
		RelationUnitsWatcherId: "1",
		Changes: params.RelationUnitsChange{
			Changed: map[string]params.UnitSettings{
				"django/0": {
					Version: 1,
				},
			},
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: `relation "hadoop:db db2:db" not found`,
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid relation tag`,
		},
	}})

	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"django:db db2:db"}},
		{"Application", []interface{}{"db2"}},
		{"Application", []interface{}{"django"}},
		{"KeyRelation", []interface{}{"hadoop:db db2:db"}},
	})

	djangoRelation.CheckCalls(c, []testing.StubCall{
		{"Endpoints", []interface{}{}},
		{"WatchUnits", []interface{}{"django"}},
	})
}

func (s *remoteRelationsSuite) TestExportEntities(c *gc.C) {
	s.st.applications["django"] = newMockApplication("django")
	result, err := s.api.ExportEntities(params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.RemoteEntityIdResult{
		Result: &params.RemoteEntityId{ModelUUID: coretesting.ModelTag.Id(), Token: "token-django"},
	})
	s.st.CheckCalls(c, []testing.StubCall{
		{"ExportLocalEntity", []interface{}{names.ApplicationTag{Name: "django"}}},
	})
}

func (s *remoteRelationsSuite) TestExportEntitiesTwice(c *gc.C) {
	s.st.applications["django"] = newMockApplication("django")
	_, err := s.api.ExportEntities(params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.api.ExportEntities(params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)
	c.Assert(result.Results[0].Error.Code, gc.Equals, params.CodeAlreadyExists)
	c.Assert(result.Results[0].Result, jc.DeepEquals, &params.RemoteEntityId{
		ModelUUID: coretesting.ModelTag.Id(), Token: "token-django"})
	s.st.CheckCalls(c, []testing.StubCall{
		{"ExportLocalEntity", []interface{}{names.ApplicationTag{Name: "django"}}},
		{"ExportLocalEntity", []interface{}{names.ApplicationTag{Name: "django"}}},
	})
}

func (s *remoteRelationsSuite) TestRelationUnitSettings(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.units["django/0"] = djangoRelationUnit
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.applications["django"] = newMockApplication("django")
	result, err := s.api.RelationUnitSettings(params.RelationUnits{
		RelationUnits: []params.RelationUnit{{Relation: "relation-db2.db#django.db", Unit: "unit-django-0"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.SettingsResult{{Settings: params.Settings{"key": "value"}}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *remoteRelationsSuite) TestRemoteApplications(c *gc.C) {
	s.st.remoteApplications["django"] = newMockRemoteApplication("django", "/u/me/django")
	result, err := s.api.RemoteApplications(params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.RemoteApplicationResult{{
		Result: &params.RemoteApplication{Name: "django", Life: "alive", ModelUUID: "model-uuid"}}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"RemoteApplication", []interface{}{"django"}},
	})
}

func (s *remoteRelationsSuite) TestRelations(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.units["django/0"] = djangoRelationUnit
	s.st.relations["db2:db django:db"] = db2Relation
	result, err := s.api.Relations(params.Entities{Entities: []params.Entity{{Tag: "relation-db2.db#django.db"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.RelationResult{{
		Id:   123,
		Life: "alive",
		Key:  "db2:db django:db",
	}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *remoteRelationsSuite) assertRegisterRemoteRelations(c *gc.C) {
	app := newMockApplication("offeredapp")
	app.eps = []state.Endpoint{{
		ApplicationName: "offeredapp",
		Relation:        charm.Relation{Name: "local"},
	}}
	s.st.applications["offeredapp"] = app
	result, err := s.api.RegisterRemoteRelations(params.RegisterRemoteRelations{
		Relations: []params.RegisterRemoteRelation{{
			ApplicationId:          params.RemoteEntityId{ModelUUID: "model-uuid", Token: "app-token"},
			RelationId:             params.RemoteEntityId{ModelUUID: "model-uuid", Token: "rel-token"},
			RemoteEndpoint:         params.RemoteEndpoint{Name: "remote"},
			OfferedApplicationName: "offeredapp",
			LocalEndpointName:      "local",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, jc.DeepEquals, []params.ErrorResult{{}})
	expectedRemoteApp := s.st.remoteApplications["remote-apptoken"]
	expectedRemoteApp.Stub = testing.Stub{} // don't care about api calls
	c.Check(expectedRemoteApp, jc.DeepEquals, &mockRemoteApplication{
		name: "remote-apptoken",
		eps:  []charm.Relation{{Name: "remote"}},
	})
	expectedRel := s.st.relations["offeredapp:local remote-apptoken:remote"]
	expectedRel.Stub = testing.Stub{} // don't care about api calls
	c.Check(expectedRel, jc.DeepEquals, &mockRelation{key: "offeredapp:local remote-apptoken:remote"})
	c.Check(s.st.remoteEntities, gc.HasLen, 2)
	c.Check(s.st.remoteEntities["remote-apptoken"], gc.Equals, "app-token")
	c.Check(s.st.remoteEntities["offeredapp:local remote-apptoken:remote"], gc.Equals, "rel-token")
}

func (s *remoteRelationsSuite) TestRegisterRemoteRelations(c *gc.C) {
	s.assertRegisterRemoteRelations(c)
}

func (s *remoteRelationsSuite) TestRegisterRemoteRelationsIdempotent(c *gc.C) {
	s.assertRegisterRemoteRelations(c)
	s.assertRegisterRemoteRelations(c)
}
