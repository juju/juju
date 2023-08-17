// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"context"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/remoterelations"
	"github.com/juju/juju/apiserver/facades/controller/remoterelations/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&remoteRelationsSuite{})

type remoteRelationsSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mocks.MockRemoteRelationsState
	ecService  *mocks.MockECService
	cc         *mocks.MockControllerConfigAPI
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
}

func (s *remoteRelationsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockRemoteRelationsState(ctrl)
	s.cc = mocks.NewMockControllerConfigAPI(ctrl)
	s.ecService = mocks.NewMockECService(ctrl)
	api, err := remoterelations.NewRemoteRelationsAPI(
		s.st, s.ecService, s.cc, s.resources, s.authorizer,
		loggo.GetLogger("test"),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
	return ctrl
}

func (s *remoteRelationsSuite) TestWatchRemoteApplications(c *gc.C) {
	defer s.setup(c).Finish()

	applicationWatcher := newMockStringsWatcher()
	applicationWatcher.changes <- []string{"db2", "hadoop"}
	s.st.EXPECT().WatchRemoteApplications().Return(applicationWatcher)
	result, err := s.api.WatchRemoteApplications(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, []string{"db2", "hadoop"})

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))
}

func (s *remoteRelationsSuite) TestWatchRemoteApplicationRelations(c *gc.C) {
	defer s.setup(c).Finish()

	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	s.st.EXPECT().WatchRemoteApplicationRelations("db2").Return(db2RelationsWatcher, nil)
	s.st.EXPECT().WatchRemoteApplicationRelations("hadoop").Return(nil, errors.NotFoundf(`application "hadoop"`))

	results, err := s.api.WatchRemoteApplicationRelations(context.Background(), params.Entities{Entities: []params.Entity{
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
}

func (s *remoteRelationsSuite) TestWatchRemoteRelations(c *gc.C) {
	defer s.setup(c).Finish()

	relationWatcher := newMockStringsWatcher()
	relationWatcher.changes <- []string{"1", "2"}
	s.st.EXPECT().WatchRemoteRelations().Return(relationWatcher)

	result, err := s.api.WatchRemoteRelations(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, []string{"1", "2"})

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))
}

func (s *remoteRelationsSuite) TestWatchLocalRelationChanges(c *gc.C) {
	defer s.setup(c).Finish()

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

	s.st.EXPECT().KeyRelation("django:db db2:db").Return(djangoRelation, nil).MinTimes(1)
	s.st.EXPECT().Application("db2").Return(nil, errors.NotFoundf(`application "db2"`)).MinTimes(1)
	s.st.EXPECT().Application("django").Return(nil, nil).MinTimes(1)

	s.st.EXPECT().GetToken(names.NewRelationTag("django:db db2:db")).Return("token-relation-django.db#db2.db", nil)
	s.st.EXPECT().GetToken(names.NewApplicationTag("django")).Return("token-application-django", nil)
	s.st.EXPECT().GetRemoteEntity("token-relation-django.db#db2.db").Return(names.NewRelationTag("django:db db2:db"), nil)

	s.st.EXPECT().KeyRelation("hadoop:db db2:db").Return(nil, errors.NotFoundf(`relation "hadoop:db db2:db"`))

	results, err := s.api.WatchLocalRelationChanges(context.Background(), params.Entities{[]params.Entity{
		{"relation-django:db#db2:db"},
		{"relation-hadoop:db#db2:db"},
		{"machine-42"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	uc := 666
	c.Assert(results.Results, jc.DeepEquals, []params.RemoteRelationWatchResult{{
		RemoteRelationWatcherId: "1",
		Changes: params.RemoteRelationChangeEvent{
			RelationToken:           "token-relation-django.db#db2.db",
			ApplicationOrOfferToken: "token-application-django",
			Macaroons:               nil,
			UnitCount:               &uc,
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
			Message: `getting relation for "hadoop:db db2:db": relation "hadoop:db db2:db" not found`,
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid relation tag`,
		},
	}})

	djangoRelation.CheckCalls(c, []testing.StubCall{
		{"Endpoints", []interface{}{}},
		{"Endpoints", []interface{}{}},
		{"WatchUnits", []interface{}{"django"}},
		{"Endpoints", []interface{}{}},
		{"ApplicationSettings", []interface{}{"django"}},
		{"Unit", []interface{}{"django/0"}},
		{"UnitCount", []interface{}{}},
	})
}

func (s *remoteRelationsSuite) TestImportRemoteEntities(c *gc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().ImportRemoteEntity(names.ApplicationTag{Name: "django"}, "token").Return(nil)

	result, err := s.api.ImportRemoteEntities(context.Background(), params.RemoteEntityTokenArgs{
		Args: []params.RemoteEntityTokenArg{
			{Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.ErrorResult{})
}

func (s *remoteRelationsSuite) TestImportRemoteEntitiesTwice(c *gc.C) {
	defer s.setup(c).Finish()

	tag := names.ApplicationTag{Name: "django"}
	s.st.EXPECT().ImportRemoteEntity(tag, "token").Return(nil)
	s.st.EXPECT().ImportRemoteEntity(tag, "token").Return(errors.AlreadyExistsf(tag.Id()))

	_, err := s.api.ImportRemoteEntities(context.Background(), params.RemoteEntityTokenArgs{
		Args: []params.RemoteEntityTokenArg{
			{Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.api.ImportRemoteEntities(context.Background(), params.RemoteEntityTokenArgs{
		Args: []params.RemoteEntityTokenArg{
			{Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)
	c.Assert(result.Results[0].Error.Code, gc.Equals, params.CodeAlreadyExists)
}

func (s *remoteRelationsSuite) TestExportEntities(c *gc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().ExportLocalEntity(names.ApplicationTag{Name: "django"}).Return("token-django", nil)

	result, err := s.api.ExportEntities(context.Background(), params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.TokenResult{
		Token: "token-django",
	})
}

func (s *remoteRelationsSuite) TestExportEntitiesTwice(c *gc.C) {
	defer s.setup(c).Finish()

	tag := names.ApplicationTag{Name: "django"}
	s.st.EXPECT().ExportLocalEntity(tag).Return("token-django", nil)
	s.st.EXPECT().ExportLocalEntity(tag).Return("token-django", errors.AlreadyExistsf(tag.Id()))

	_, err := s.api.ExportEntities(context.Background(), params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.api.ExportEntities(context.Background(), params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)
	c.Assert(result.Results[0].Error.Code, gc.Equals, params.CodeAlreadyExists)
	c.Assert(result.Results[0].Token, gc.Equals, "token-django")
}

func (s *remoteRelationsSuite) TestGetTokens(c *gc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().GetToken(names.NewApplicationTag("django")).Return("token-application-django", nil)

	result, err := s.api.GetTokens(context.Background(), params.GetTokenArgs{
		Args: []params.GetTokenArg{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.StringResult{Result: "token-application-django"})
}

func (s *remoteRelationsSuite) TestSaveMacaroons(c *gc.C) {
	defer s.setup(c).Finish()

	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	relTag := names.NewRelationTag("mysql:db wordpress:db")
	s.st.EXPECT().SaveMacaroon(relTag, mac).Return(nil)

	result, err := s.api.SaveMacaroons(context.Background(), params.EntityMacaroonArgs{
		Args: []params.EntityMacaroonArg{{Tag: relTag.String(), Macaroon: mac}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *remoteRelationsSuite) TestRemoteApplications(c *gc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().RemoteApplication("django").Return(newMockRemoteApplication("django", "me/model.riak"), nil)

	result, err := s.api.RemoteApplications(context.Background(), params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	mac, err := jujutesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.RemoteApplicationResult{{
		Result: &params.RemoteApplication{
			Name:           "django",
			OfferUUID:      "django-uuid",
			ConsumeVersion: 666,
			Life:           "alive",
			ModelUUID:      "model-uuid",
			Macaroon:       mac,
		},
	}})
}

func (s *remoteRelationsSuite) TestRelations(c *gc.C) {
	defer s.setup(c).Finish()

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
	app := newMockApplication("django")
	remoteApp := newMockRemoteApplication("db2", "url")

	s.st.EXPECT().KeyRelation("db2:db django:db").Return(db2Relation, nil)
	s.st.EXPECT().RemoteApplication("django").Return(nil, errors.NotFoundf(`saas application "django"`))
	s.st.EXPECT().Application("django").Return(app, nil)
	s.st.EXPECT().RemoteApplication("db2").Return(remoteApp, nil)

	result, err := s.api.Relations(context.Background(), params.Entities{Entities: []params.Entity{{Tag: "relation-db2.db#django.db"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.RemoteRelationResult{{
		Result: &params.RemoteRelation{
			Id:                    123,
			Life:                  "alive",
			Suspended:             true,
			Key:                   "db2:db django:db",
			UnitCount:             666,
			RemoteApplicationName: "db2",
			RemoteEndpointName:    "data",
			ApplicationName:       "django",
			SourceModelUUID:       "model-uuid",
			Endpoint: params.RemoteEndpoint{
				Name:      "db",
				Role:      "provides",
				Interface: "db2",
			},
		},
	}})
}

func (s *remoteRelationsSuite) TestConsumeRemoteRelationChange(c *gc.C) {
	defer s.setup(c).Finish()

	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.remoteUnits["django/0"] = djangoRelationUnit

	change := params.RemoteRelationChangeEvent{
		RelationToken:           "rel-token",
		ApplicationOrOfferToken: "app-token",
		Life:                    life.Alive,
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   0,
			Settings: map[string]interface{}{"foo": "bar"},
		}},
	}
	changes := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{change},
	}

	s.st.EXPECT().GetRemoteEntity("rel-token").Return(names.NewRelationTag("db2:db django:db"), nil)
	s.st.EXPECT().KeyRelation("db2:db django:db").Return(db2Relation, nil)
	s.st.EXPECT().GetRemoteEntity("app-token").Return(names.NewApplicationTag("django"), nil)
	s.st.EXPECT().ModelUUID().AnyTimes()

	result, err := s.api.ConsumeRemoteRelationChanges(context.Background(), changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)

	settings, err := db2Relation.remoteUnits["django/0"].Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func ptr[T any](v T) *T {
	return &v
}

func (s *remoteRelationsSuite) TestConsumeRelationResumePermission(c *gc.C) {
	defer s.setup(c).Finish()

	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.suspended = true
	db2Relation.key = "db2:db django:db"
	db2Relation.remoteUnits["django/0"] = djangoRelationUnit
	offerConn := &mockOfferConnection{offerUUID: "offer-uuid", username: "fred"}

	change := params.RemoteRelationChangeEvent{
		RelationToken:           "rel-token",
		ApplicationOrOfferToken: "app-token",
		Life:                    life.Alive,
		Suspended:               ptr(false),
	}
	changes := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{change},
	}

	s.st.EXPECT().GetRemoteEntity("app-token").Return(names.NewApplicationTag("db2"), nil)
	s.st.EXPECT().GetRemoteEntity("rel-token").Return(names.NewRelationTag(db2Relation.key), nil)
	s.st.EXPECT().KeyRelation(db2Relation.key).Return(db2Relation, nil)
	s.st.EXPECT().ModelUUID().AnyTimes()
	s.st.EXPECT().ControllerTag().Return(coretesting.ControllerTag)
	s.st.EXPECT().ModelTag().Return(coretesting.ModelTag)
	s.st.EXPECT().OfferConnectionForRelation(db2Relation.key).Return(offerConn, nil)
	s.st.EXPECT().ApplicationOfferForUUID("offer-uuid").Return(&crossmodel.ApplicationOffer{ApplicationName: "django"}, nil)

	result, err := s.api.ConsumeRemoteRelationChanges(context.Background(), changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, "permission denied")
}

func (s *remoteRelationsSuite) TestSetRemoteApplicationsStatus(c *gc.C) {
	defer s.setup(c).Finish()

	remoteApp := newMockRemoteApplication("db2", "url")
	entity := names.NewApplicationTag("db2")
	s.st.EXPECT().RemoteApplication("db2").Return(remoteApp, nil)

	result, err := s.api.SetRemoteApplicationsStatus(
		context.Background(),
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
	defer s.setup(c).Finish()

	remoteApp := newMockRemoteApplication("db2", "url")
	entity := names.NewApplicationTag("db2")
	s.st.EXPECT().RemoteApplication("db2").Return(remoteApp, nil)
	s.st.EXPECT().ApplyOperation(&mockOperation{message: "killer whales"}).Return(nil)

	result, err := s.api.SetRemoteApplicationsStatus(
		context.Background(),
		params.SetStatus{Entities: []params.EntityStatusArgs{{
			Tag:    entity.String(),
			Status: "terminated",
			Info:   "killer whales",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(remoteApp.terminated, gc.Equals, true)
}

func (s *remoteRelationsSuite) TestUpdateControllersForModels(c *gc.C) {
	defer s.setup(c).Finish()

	mod1 := utils.MustNewUUID().String()
	c1Tag := names.NewControllerTag(utils.MustNewUUID().String())
	mod2 := utils.MustNewUUID().String()
	c2Tag := names.NewControllerTag(utils.MustNewUUID().String())

	c1 := crossmodel.ControllerInfo{
		ControllerTag: c1Tag,
		Alias:         "alias1",
		Addrs:         []string{"1.1.1.1:1"},
		CACert:        "cert1",
		ModelUUIDs:    []string{mod1},
	}
	c2 := crossmodel.ControllerInfo{
		ControllerTag: c2Tag,
		Alias:         "alias2",
		Addrs:         []string{"2.2.2.2:2"},
		CACert:        "cert2",
		ModelUUIDs:    []string{mod2},
	}

	s.ecService.EXPECT().UpdateExternalController(
		gomock.Any(),
		c1,
	).Return(errors.New("whack"))
	s.ecService.EXPECT().UpdateExternalController(
		gomock.Any(),
		c2,
	).Return(nil)

	res, err := s.api.UpdateControllersForModels(
		context.Background(),
		params.UpdateControllersForModelsParams{
			Changes: []params.UpdateControllerForModel{
				{
					ModelTag: names.NewModelTag(mod1).String(),
					Info: params.ExternalControllerInfo{
						ControllerTag: c1Tag.String(),
						Alias:         "alias1",
						Addrs:         []string{"1.1.1.1:1"},
						CACert:        "cert1",
					},
				},
				{
					ModelTag: names.NewModelTag(mod2).String(),
					Info: params.ExternalControllerInfo{
						ControllerTag: c2Tag.String(),
						Alias:         "alias2",
						Addrs:         []string{"2.2.2.2:2"},
						CACert:        "cert2",
					},
				},
			},
		})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 2)
	c.Assert(res.Results[0].Error.Message, gc.Equals, "whack")
	c.Assert(res.Results[1].Error, gc.IsNil)
}

func (s *remoteRelationsSuite) TestConsumeRemoteSecretChanges(c *gc.C) {
	defer s.setup(c).Finish()

	uri := secrets.NewURI()
	change := params.SecretRevisionChange{
		URI:      uri.String(),
		Revision: 666,
	}
	changes := params.LatestSecretRevisionChanges{
		Changes: []params.SecretRevisionChange{change},
	}

	op := &mockOperation{message: "killer whales"}
	s.st.EXPECT().UpdateSecretConsumerOperation(uri, 666).Return(op, nil)
	s.st.EXPECT().ApplyOperation(op).Return(nil)

	result, err := s.api.ConsumeRemoteSecretChanges(context.Background(), changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
}
