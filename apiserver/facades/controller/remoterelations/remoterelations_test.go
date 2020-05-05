// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/remoterelations"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&remoteRelationsSuite{})

type remoteRelationsSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	api        *remoterelations.API
}

func (s *remoteRelationsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState()
	api, err := remoterelations.NewRemoteRelationsAPI(s.st, common.NewControllerConfig(s.st), s.resources, s.authorizer)
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

	results, err := s.api.WatchRemoteApplicationRelations(params.Entities{Entities: []params.Entity{
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

func (s *remoteRelationsSuite) TestWatchRemoteRelations(c *gc.C) {
	relationsIds := []string{"1", "2"}
	s.st.remoteRelationsWatcher.changes <- relationsIds
	result, err := s.api.WatchRemoteRelations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, relationsIds)

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))
}

func (s *remoteRelationsSuite) TestWatchLocalRelationUnits(c *gc.C) {
	djangoRelationUnitsWatcher := newMockRelationUnitsWatcher()
	djangoRelationUnitsWatcher.changes <- watcher.RelationUnitsChange{
		Changed:    map[string]watcher.UnitSettings{"django/0": {Version: 1}},
		AppChanged: map[string]int64{"django": 0},
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

	// WatchLocalRelationUnits has been removed from the V2 API.
	api := &remoterelations.APIv1{s.api}
	results, err := api.WatchLocalRelationUnits(params.Entities{[]params.Entity{
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
			AppChanged: map[string]int64{
				"django": 0,
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

func (s *remoteRelationsSuite) TestWatchLocalRelationChanges(c *gc.C) {
	djangoRelationUnitsWatcher := newMockRelationUnitsWatcher()
	djangoRelationUnitsWatcher.changes <- watcher.RelationUnitsChange{
		Changed:    map[string]watcher.UnitSettings{"django/0": {Version: 1}},
		AppChanged: map[string]int64{"django": 0},
		Departed:   []string{"django/1", "django/2"},
	}
	djangoRelation := newMockRelation(123)
	ru1 := newMockRelationUnit()

	ru1.settings["barnett"] = "depreston"
	djangoRelation.units["django/0"] = ru1

	djangoRelation.endpointUnitsWatchers["django"] = djangoRelationUnitsWatcher
	djangoRelation.endpoints = []state.Endpoint{{
		ApplicationName: "db2",
	}, {
		ApplicationName: "django",
	}}
	djangoRelation.appSettings["django"] = map[string]interface{}{
		"sunday": "roast",
	}

	s.st.relations["django:db db2:db"] = djangoRelation
	s.st.applications["django"] = newMockApplication("django")

	s.st.remoteEntities[names.NewRelationTag("django:db db2:db")] = "token-relation-django.db#db2.db"
	s.st.remoteEntities[names.NewApplicationTag("django")] = "token-application-django"

	results, err := s.api.WatchLocalRelationChanges(params.Entities{[]params.Entity{
		{"relation-django:db#db2:db"},
		{"relation-hadoop:db#db2:db"},
		{"machine-42"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.RemoteRelationWatchResult{{
		RemoteRelationWatcherId: "1",
		Changes: params.RemoteRelationChangeEvent{
			RelationToken:    "token-relation-django.db#db2.db",
			ApplicationToken: "token-application-django",
			Macaroons:        nil,
			ApplicationSettings: map[string]interface{}{
				"sunday": "roast",
			},
			ChangedUnits: []params.RemoteRelationUnitChange{{
				UnitId: 0,
				Settings: map[string]interface{}{
					"barnett": "depreston",
				},
			}},
			DepartedUnits: []int{1, 2},
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
		{"GetToken", []interface{}{names.NewRelationTag("django:db db2:db")}},
		{"GetToken", []interface{}{names.NewApplicationTag("django")}},
		{"KeyRelation", []interface{}{"django:db db2:db"}},
		{"Application", []interface{}{"db2"}},
		{"Application", []interface{}{"django"}},
		{"GetRemoteEntity", []interface{}{"token-relation-django.db#db2.db"}},
		{"KeyRelation", []interface{}{"django:db db2:db"}},
		{"Application", []interface{}{"db2"}},
		{"Application", []interface{}{"django"}},
		{"KeyRelation", []interface{}{"hadoop:db db2:db"}},
	})

	djangoRelation.CheckCalls(c, []testing.StubCall{
		{"Endpoints", []interface{}{}},
		{"Endpoints", []interface{}{}},
		{"WatchUnits", []interface{}{"django"}},
		{"Endpoints", []interface{}{}},
		{"ApplicationSettings", []interface{}{"django"}},
		{"Unit", []interface{}{"django/0"}},
	})
}

func (s *remoteRelationsSuite) TestImportRemoteEntities(c *gc.C) {
	result, err := s.api.ImportRemoteEntities(params.RemoteEntityTokenArgs{
		Args: []params.RemoteEntityTokenArg{
			{Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.ErrorResult{})
	s.st.CheckCalls(c, []testing.StubCall{
		{"ImportRemoteEntity", []interface{}{names.ApplicationTag{Name: "django"}, "token"}},
	})
}

func (s *remoteRelationsSuite) TestImportRemoteEntitiesTwice(c *gc.C) {
	_, err := s.api.ImportRemoteEntities(params.RemoteEntityTokenArgs{
		Args: []params.RemoteEntityTokenArg{
			{Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.api.ImportRemoteEntities(params.RemoteEntityTokenArgs{
		Args: []params.RemoteEntityTokenArg{
			{Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)
	c.Assert(result.Results[0].Error.Code, gc.Equals, params.CodeAlreadyExists)
	s.st.CheckCalls(c, []testing.StubCall{
		{"ImportRemoteEntity", []interface{}{names.ApplicationTag{Name: "django"}, "token"}},
		{"ImportRemoteEntity", []interface{}{names.ApplicationTag{Name: "django"}, "token"}},
	})
}

func (s *remoteRelationsSuite) TestExportEntities(c *gc.C) {
	s.st.applications["django"] = newMockApplication("django")
	result, err := s.api.ExportEntities(params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.TokenResult{
		Token: "token-django",
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
	c.Assert(result.Results[0].Token, gc.Equals, "token-django")
	s.st.CheckCalls(c, []testing.StubCall{
		{"ExportLocalEntity", []interface{}{names.ApplicationTag{Name: "django"}}},
		{"ExportLocalEntity", []interface{}{names.ApplicationTag{Name: "django"}}},
	})
}

func (s *remoteRelationsSuite) TestGetTokens(c *gc.C) {
	s.st.applications["django"] = newMockApplication("django")
	result, err := s.api.GetTokens(params.GetTokenArgs{
		Args: []params.GetTokenArg{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.StringResult{Result: "token-application-django"})
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetToken", []interface{}{names.NewApplicationTag("django")}},
	})
}

func (s *remoteRelationsSuite) TestSaveMacaroons(c *gc.C) {
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	relTag := names.NewRelationTag("mysql:db wordpress:db")
	result, err := s.api.SaveMacaroons(params.EntityMacaroonArgs{
		Args: []params.EntityMacaroonArg{{Tag: relTag.String(), Macaroon: mac}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	s.st.CheckCalls(c, []testing.StubCall{
		{"SaveMacaroon", []interface{}{relTag, mac.Id()}},
	})
}

func (s *remoteRelationsSuite) TestRelationUnitSettings(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.units["django/0"] = djangoRelationUnit
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.applications["django"] = newMockApplication("django")
	// RelationUnitSettings has been removed from the V2 API.
	api := &remoterelations.APIv1{s.api}
	result, err := api.RelationUnitSettings(params.RelationUnits{
		RelationUnits: []params.RelationUnit{{Relation: "relation-db2.db#django.db", Unit: "unit-django-0"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.SettingsResult{{Settings: params.Settings{"key": "value"}}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *remoteRelationsSuite) TestRemoteApplications(c *gc.C) {
	s.st.remoteApplications["django"] = newMockRemoteApplication("django", "me/model.riak")
	result, err := s.api.RemoteApplications(params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.RemoteApplicationResult{{
		Result: &params.RemoteApplication{
			Name:      "django",
			OfferUUID: "django-uuid",
			Life:      "alive",
			ModelUUID: "model-uuid",
			Macaroon:  mac,
		}}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"RemoteApplication", []interface{}{"django"}},
	})
}

func (s *remoteRelationsSuite) TestRelations(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.suspended = true
	db2Relation.units["django/0"] = djangoRelationUnit
	db2Relation.endpoints = []state.Endpoint{
		{
			ApplicationName: "django",
			Relation: charm.Relation{
				Name:      "db",
				Interface: "db2",
				Role:      "provides",
				Limit:     1,
				Scope:     charm.ScopeGlobal,
			},
		}, {
			ApplicationName: "db2",
			Relation: charm.Relation{
				Name:      "data",
				Interface: "db2",
				Role:      "requires",
				Limit:     1,
				Scope:     charm.ScopeGlobal,
			},
		},
	}
	s.st.relations["db2:db django:db"] = db2Relation
	app := newMockApplication("django")
	s.st.applications["django"] = app
	remoteApp := newMockRemoteApplication("db2", "url")
	s.st.remoteApplications["db2"] = remoteApp
	result, err := s.api.Relations(params.Entities{Entities: []params.Entity{{Tag: "relation-db2.db#django.db"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.RemoteRelationResult{{
		Result: &params.RemoteRelation{
			Id:                    123,
			Life:                  "alive",
			Suspended:             true,
			Key:                   "db2:db django:db",
			RemoteApplicationName: "db2",
			RemoteEndpointName:    "data",
			ApplicationName:       "django",
			SourceModelUUID:       "model-uuid",
			Endpoint: params.RemoteEndpoint{
				Name:      "db",
				Role:      "provides",
				Interface: "db2",
			}},
	}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"RemoteApplication", []interface{}{"django"}},
		{"Application", []interface{}{"django"}},
		{"RemoteApplication", []interface{}{"db2"}},
	})
}

func (s *remoteRelationsSuite) TestConsumeRemoteRelationChange(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.remoteUnits["django/0"] = djangoRelationUnit
	s.st.relations["db2:db django:db"] = db2Relation
	app := newMockApplication("django")
	s.st.applications["django"] = app
	remoteApp := newMockRemoteApplication("db2", "url")
	s.st.remoteApplications["db2"] = remoteApp

	_, err := s.api.ImportRemoteEntities(params.RemoteEntityTokenArgs{
		Args: []params.RemoteEntityTokenArg{
			{Tag: "application-django", Token: "app-token"},
			{Tag: "relation-db2:db#django:db", Token: "rel-token"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	s.st.ResetCalls()

	change := params.RemoteRelationChangeEvent{
		RelationToken:    "rel-token",
		ApplicationToken: "app-token",
		Life:             life.Alive,
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   0,
			Settings: map[string]interface{}{"foo": "bar"},
		}},
	}
	changes := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{change},
	}
	result, err := s.api.ConsumeRemoteRelationChanges(changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)

	settings, err := db2Relation.remoteUnits["django/0"].Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, map[string]interface{}{"foo": "bar"})

	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"rel-token"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"app-token"}},
	})
}

func (s *remoteRelationsSuite) TestControllerAPIInfoForModels(c *gc.C) {
	controllerInfo := &mockControllerInfo{
		uuid: "some uuid",
		info: crossmodel.ControllerInfo{
			Addrs:  []string{"1.2.3.4/32"},
			CACert: coretesting.CACert,
		},
	}
	s.st.controllerInfo[coretesting.ModelTag.Id()] = controllerInfo
	result, err := s.api.ControllerAPIInfoForModels(
		params.Entities{Entities: []params.Entity{{
			Tag: coretesting.ModelTag.String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Addresses, jc.SameContents, []string{"1.2.3.4/32"})
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].CACert, gc.Equals, coretesting.CACert)
}

func (s *remoteRelationsSuite) TestSetRemoteApplicationsStatus(c *gc.C) {
	remoteApp := newMockRemoteApplication("db2", "url")
	s.st.remoteApplications["db2"] = remoteApp
	entity := names.NewApplicationTag("db2")
	result, err := s.api.SetRemoteApplicationsStatus(
		params.SetStatus{Entities: []params.EntityStatusArgs{{
			Tag:    entity.String(),
			Status: "blocked",
			Info:   "a message",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(remoteApp.status, gc.Equals, status.Blocked)
	c.Assert(remoteApp.message, gc.Equals, "a message")
}

func (s *remoteRelationsSuite) TestSetRemoteApplicationsStatusTerminated(c *gc.C) {
	remoteApp := newMockRemoteApplication("db2", "url")
	s.st.remoteApplications["db2"] = remoteApp
	entity := names.NewApplicationTag("db2")
	result, err := s.api.SetRemoteApplicationsStatus(
		params.SetStatus{Entities: []params.EntityStatusArgs{{
			Tag:    entity.String(),
			Status: "terminated",
			Info:   "killer whales",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(remoteApp.terminated, gc.Equals, true)
	s.st.CheckCallNames(c, "RemoteApplication", "ApplyOperation")
	s.st.CheckCall(c, 1, "ApplyOperation", &mockOperation{message: "killer whales"})
}

func (s *remoteRelationsSuite) TestUpdateControllersForModels(c *gc.C) {
	mod1 := utils.MustNewUUID().String()
	c1 := names.NewControllerTag(utils.MustNewUUID().String())
	mod2 := utils.MustNewUUID().String()
	c2 := names.NewControllerTag(utils.MustNewUUID().String())

	// Return an error for the first of the arguments.
	s.st.SetErrors(errors.New("whack"))

	res, err := s.api.UpdateControllersForModels(params.UpdateControllersForModelsParams{
		Changes: []params.UpdateControllerForModel{
			{
				ModelTag: names.NewModelTag(mod1).String(),
				Info: params.ExternalControllerInfo{
					ControllerTag: c1.String(),
					Alias:         "alias1",
					Addrs:         []string{"1.1.1.1:1"},
					CACert:        "cert1",
				},
			},
			{
				ModelTag: names.NewModelTag(mod2).String(),
				Info: params.ExternalControllerInfo{
					ControllerTag: c2.String(),
					Alias:         "alias2",
					Addrs:         []string{"2.2.2.2:2"},
					CACert:        "cert2",
				},
			},
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "UpdateControllerForModel", "UpdateControllerForModel")

	s.st.CheckCall(c, 0, "UpdateControllerForModel", crossmodel.ControllerInfo{
		ControllerTag: c1,
		Alias:         "alias1",
		Addrs:         []string{"1.1.1.1:1"},
		CACert:        "cert1",
	}, mod1)

	s.st.CheckCall(c, 1, "UpdateControllerForModel", crossmodel.ControllerInfo{
		ControllerTag: c2,
		Alias:         "alias2",
		Addrs:         []string{"2.2.2.2:2"},
		CACert:        "cert2",
	}, mod2)

	c.Assert(res.Results, gc.HasLen, 2)
	c.Assert(res.Results[0].Error.Message, gc.Equals, "whack")
	c.Assert(res.Results[1].Error, gc.IsNil)
}
