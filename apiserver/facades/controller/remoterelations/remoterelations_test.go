// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/remoterelations"
	"github.com/juju/juju/apiserver/facades/controller/remoterelations/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

func TestRemoteRelationsSuite(t *stdtesting.T) {
	tc.Run(t, &remoteRelationsSuite{})
}

type remoteRelationsSuite struct {
	coretesting.BaseSuite

	resources     *common.Resources
	authorizer    *apiservertesting.FakeAuthorizer
	st            *mocks.MockRemoteRelationsState
	ecService     *mocks.MockExternalControllerService
	secretService *mocks.MockSecretService
	cc            *mocks.MockControllerConfigAPI
	api           *remoterelations.API
}

func (s *remoteRelationsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
}

func (s *remoteRelationsSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockRemoteRelationsState(ctrl)
	s.cc = mocks.NewMockControllerConfigAPI(ctrl)
	s.ecService = mocks.NewMockExternalControllerService(ctrl)
	s.secretService = mocks.NewMockSecretService(ctrl)
	modelID := modeltesting.GenModelUUID(c)
	api, err := remoterelations.NewRemoteRelationsAPI(
		modelID,
		s.st,
		s.ecService,
		s.secretService,
		s.cc,
		s.resources,
		s.authorizer,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	s.api = api
	return ctrl
}

func (s *remoteRelationsSuite) TestWatchStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
	- Watch remote applications
    - Watch remote applications relations
    - Watch remote relations`)
}

func (s *remoteRelationsSuite) TestWatchLocalRelationChanges(c *tc.C) {
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
	djangoRelation.endpoints = []relation.Endpoint{{
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

	results, err := s.api.WatchLocalRelationChanges(c.Context(), params.Entities{[]params.Entity{
		{"relation-django:db#db2:db"},
		{"relation-hadoop:db#db2:db"},
		{"machine-42"},
	}})
	c.Assert(err, tc.ErrorIsNil)
	uc := 666
	c.Assert(results.Results, tc.DeepEquals, []params.RemoteRelationWatchResult{{
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

	djangoRelation.CheckCalls(c, []testhelpers.StubCall{
		{"Endpoints", []interface{}{}},
		{"Endpoints", []interface{}{}},
		{"WatchUnits", []interface{}{"django"}},
		{"Endpoints", []interface{}{}},
		{"ApplicationSettings", []interface{}{"django"}},
		{"Unit", []interface{}{"django/0"}},
		{"UnitCount", []interface{}{}},
	})
}

func (s *remoteRelationsSuite) TestImportRemoteEntities(c *tc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().ImportRemoteEntity(names.ApplicationTag{Name: "django"}, "token").Return(nil)

	result, err := s.api.ImportRemoteEntities(c.Context(), params.RemoteEntityTokenArgs{
		Args: []params.RemoteEntityTokenArg{
			{Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0], tc.DeepEquals, params.ErrorResult{})
}

func (s *remoteRelationsSuite) TestImportRemoteEntitiesTwice(c *tc.C) {
	defer s.setup(c).Finish()

	tag := names.ApplicationTag{Name: "django"}
	s.st.EXPECT().ImportRemoteEntity(tag, "token").Return(nil)
	s.st.EXPECT().ImportRemoteEntity(tag, "token").Return(errors.AlreadyExistsf(tag.Id()))

	_, err := s.api.ImportRemoteEntities(c.Context(), params.RemoteEntityTokenArgs{
		Args: []params.RemoteEntityTokenArg{
			{Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, tc.ErrorIsNil)
	result, err := s.api.ImportRemoteEntities(c.Context(), params.RemoteEntityTokenArgs{
		Args: []params.RemoteEntityTokenArg{
			{Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeAlreadyExists)
}

func (s *remoteRelationsSuite) TestExportEntities(c *tc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().ExportLocalEntity(names.ApplicationTag{Name: "django"}).Return("token-django", nil)

	result, err := s.api.ExportEntities(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0], tc.DeepEquals, params.TokenResult{
		Token: "token-django",
	})
}

func (s *remoteRelationsSuite) TestExportEntitiesTwice(c *tc.C) {
	defer s.setup(c).Finish()

	tag := names.ApplicationTag{Name: "django"}
	s.st.EXPECT().ExportLocalEntity(tag).Return("token-django", nil)
	s.st.EXPECT().ExportLocalEntity(tag).Return("token-django", errors.AlreadyExistsf(tag.Id()))

	_, err := s.api.ExportEntities(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, tc.ErrorIsNil)
	result, err := s.api.ExportEntities(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeAlreadyExists)
	c.Assert(result.Results[0].Token, tc.Equals, "token-django")
}

func (s *remoteRelationsSuite) TestGetTokens(c *tc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().GetToken(names.NewApplicationTag("django")).Return("token-application-django", nil)

	result, err := s.api.GetTokens(c.Context(), params.GetTokenArgs{
		Args: []params.GetTokenArg{{Tag: "application-django"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0], tc.DeepEquals, params.StringResult{Result: "token-application-django"})
}

func (s *remoteRelationsSuite) TestSaveMacaroons(c *tc.C) {
	defer s.setup(c).Finish()

	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	relTag := names.NewRelationTag("mysql:db wordpress:db")
	s.st.EXPECT().SaveMacaroon(relTag, mac).Return(nil)

	result, err := s.api.SaveMacaroons(c.Context(), params.EntityMacaroonArgs{
		Args: []params.EntityMacaroonArg{{Tag: relTag.String(), Macaroon: mac}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *remoteRelationsSuite) TestRemoteApplications(c *tc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().RemoteApplication("django").Return(newMockRemoteApplication("django", "me/model.riak"), nil)

	result, err := s.api.RemoteApplications(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, tc.ErrorIsNil)
	mac, err := jujutesting.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.DeepEquals, []params.RemoteApplicationResult{{
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

func (s *remoteRelationsSuite) TestRelations(c *tc.C) {
	defer s.setup(c).Finish()

	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.suspended = true
	db2Relation.units["django/0"] = djangoRelationUnit
	db2Relation.endpoints = []relation.Endpoint{
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

	result, err := s.api.Relations(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "relation-db2.db#django.db"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.DeepEquals, []params.RemoteRelationResult{{
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

func (s *remoteRelationsSuite) TestConsumeRemoteRelationChange(c *tc.C) {
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

	result, err := s.api.ConsumeRemoteRelationChanges(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.IsNil)

	settings, err := db2Relation.remoteUnits["django/0"].Settings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func ptr[T any](v T) *T {
	return &v
}

func (s *remoteRelationsSuite) TestConsumeRelationResumePermission(c *tc.C) {
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
	s.st.EXPECT().ControllerTag().Return(coretesting.ControllerTag)
	s.st.EXPECT().OfferConnectionForRelation(db2Relation.key).Return(offerConn, nil)

	result, err := s.api.ConsumeRemoteRelationChanges(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorMatches, "permission denied")
}

func (s *remoteRelationsSuite) TestSetRemoteApplicationsStatus(c *tc.C) {
	defer s.setup(c).Finish()

	remoteApp := newMockRemoteApplication("db2", "url")
	entity := names.NewApplicationTag("db2")
	s.st.EXPECT().RemoteApplication("db2").Return(remoteApp, nil)

	result, err := s.api.SetRemoteApplicationsStatus(
		c.Context(),
		params.SetStatus{Entities: []params.EntityStatusArgs{{
			Tag:    entity.String(),
			Status: "blocked",
			Info:   "a message",
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(remoteApp.status, tc.Equals, status.Blocked)
	c.Assert(remoteApp.message, tc.Equals, "a message")
}

func (s *remoteRelationsSuite) TestSetRemoteApplicationsStatusTerminated(c *tc.C) {
	defer s.setup(c).Finish()

	remoteApp := newMockRemoteApplication("db2", "url")
	entity := names.NewApplicationTag("db2")
	s.st.EXPECT().RemoteApplication("db2").Return(remoteApp, nil)
	s.st.EXPECT().ApplyOperation(&mockOperation{message: "killer whales"}).Return(nil)

	result, err := s.api.SetRemoteApplicationsStatus(
		c.Context(),
		params.SetStatus{Entities: []params.EntityStatusArgs{{
			Tag:    entity.String(),
			Status: "terminated",
			Info:   "killer whales",
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(remoteApp.terminated, tc.Equals, true)
}

func (s *remoteRelationsSuite) TestUpdateControllersForModels(c *tc.C) {
	defer s.setup(c).Finish()

	mod1 := uuid.MustNewUUID().String()
	c1Tag := names.NewControllerTag(uuid.MustNewUUID().String())
	mod2 := uuid.MustNewUUID().String()
	c2Tag := names.NewControllerTag(uuid.MustNewUUID().String())

	c1 := crossmodel.ControllerInfo{
		ControllerUUID: c1Tag.Id(),
		Alias:          "alias1",
		Addrs:          []string{"1.1.1.1:1"},
		CACert:         "cert1",
		ModelUUIDs:     []string{mod1},
	}
	c2 := crossmodel.ControllerInfo{
		ControllerUUID: c2Tag.Id(),
		Alias:          "alias2",
		Addrs:          []string{"2.2.2.2:2"},
		CACert:         "cert2",
		ModelUUIDs:     []string{mod2},
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
		c.Context(),
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

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 2)
	c.Assert(res.Results[0].Error.Message, tc.Equals, "whack")
	c.Assert(res.Results[1].Error, tc.IsNil)
}

func (s *remoteRelationsSuite) TestConsumeRemoteSecretChanges(c *tc.C) {
	defer s.setup(c).Finish()

	uri := secrets.NewURI()
	change := params.SecretRevisionChange{
		URI:            uri.String(),
		LatestRevision: 666,
	}
	changes := params.LatestSecretRevisionChanges{
		Changes: []params.SecretRevisionChange{change},
	}

	s.secretService.EXPECT().UpdateRemoteSecretRevision(gomock.Any(), uri, 666).Return(nil)

	result, err := s.api.ConsumeRemoteSecretChanges(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.IsNil)
}
