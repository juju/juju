// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"bytes"
	"context"
	"regexp"
	"sync"
	stdtesting "testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

func TestCrossmodelRelationsSuite(t *stdtesting.T) {
	tc.Run(t, &crossmodelRelationsSuite{})
}

type crossmodelRelationsSuite struct {
	coretesting.BaseSuite

	modelConfigService *MockModelConfigService
	statusService      *MockStatusService

	resources     *common.Resources
	authorizer    *apiservertesting.FakeAuthorizer
	st            *mockState
	secretService *mockSecretService
	bakery        *mockBakeryService
	api           *crossmodelrelations.CrossModelRelationsAPIv3

	watchedRelations       params.Entities
	watchedOffers          []string
	watchedSecretConsumers []string
}

func (s *crossmodelRelationsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.statusService = NewMockStatusService(ctrl)

	return ctrl
}

func (s *crossmodelRelationsSuite) setupAPI(c *tc.C) {
	s.bakery = &mockBakeryService{}
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState()
	fw := &mockFirewallState{}
	egressAddressWatcher := func(ctx context.Context, _ facade.Resources, fws firewall.State, modelConfigService firewall.ModelConfigService, relations params.Entities) (params.StringsWatchResults, error) {
		c.Assert(fw, tc.Equals, fws)
		s.watchedRelations = relations
		return params.StringsWatchResults{Results: make([]params.StringsWatchResult, len(relations.Entities))}, nil
	}
	relationStatusWatcher := func(st crossmodelrelations.CrossModelRelationsState, tag names.RelationTag) (state.StringsWatcher, error) {
		c.Assert(s.st, tc.Equals, st)
		s.watchedRelations = params.Entities{Entities: []params.Entity{{Tag: tag.String()}}}
		w := &mockRelationStatusWatcher{
			mockWatcher: &mockWatcher{
				mu:      sync.Mutex{},
				stopped: make(chan struct{}, 1),
			},
			changes: make(chan []string, 1),
		}
		w.changes <- []string{"db2:db django:db"}
		return w, nil
	}
	offerStatusWatcher := func(_ context.Context, st crossmodelrelations.CrossModelRelationsState, offerUUID string) (crossmodelrelations.OfferWatcher, error) {
		c.Assert(s.st, tc.Equals, st)
		s.watchedOffers = []string{offerUUID}
		w := &mockOfferStatusWatcher{
			mockWatcher: &mockWatcher{
				mu:      sync.Mutex{},
				stopped: make(chan struct{}, 1),
			},
			offerUUID: offerUUID,
			offerName: "mysql",
			changes:   make(chan struct{}, 1),
		}
		w.changes <- struct{}{}
		return w, nil
	}
	consumedSecretsWatcher := func(_ context.Context, _ crossmodelrelations.SecretService, appName string) (watcher.StringsWatcher, error) {
		s.watchedSecretConsumers = []string{appName}
		w := &mockSecretsWatcher{
			mockWatcher: &mockWatcher{
				mu:      sync.Mutex{},
				stopped: make(chan struct{}, 1),
			},
			changes: make(chan []string, 1),
		}
		w.changes <- []string{"9m4e2mr0ui3e8a215n4g"}
		return w, nil
	}
	var err error
	thirdPartyKey := bakery.MustGenerateKey()
	authContext, err := commoncrossmodel.NewAuthContext(
		s.st, nil, coretesting.ModelTag, thirdPartyKey,
		commoncrossmodel.NewOfferBakeryForTest(s.bakery, clock.WallClock),
	)
	c.Assert(err, tc.ErrorIsNil)
	s.secretService = newMockSecretService()
	api, err := crossmodelrelations.NewCrossModelRelationsAPI(
		model.UUID(coretesting.ModelTag.Id()),
		s.st, fw, s.resources, s.authorizer,
		authContext, s.secretService, s.modelConfigService, s.statusService, egressAddressWatcher, relationStatusWatcher,
		offerStatusWatcher, consumedSecretsWatcher,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	s.api = api
}

func (s *crossmodelRelationsSuite) assertPublishRelationsChanges(c *tc.C, lifeValue life.Value, suspendedReason string, forceCleanup bool) {
	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.remoteEntities[names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")] = "token-db2"
	s.st.offers["f47ac10b-58cc-4372-a567-0e02b2c3d479"] = &crossmodel.ApplicationOffer{
		OfferName: "db2-offer", ApplicationName: "db2"}
	rel := newMockRelation(1)
	ru1 := newMockRelationUnit()
	ru2 := newMockRelationUnit()
	rel.units["db2/1"] = ru1
	rel.units["db2/2"] = ru2
	s.st.relations["db2:db django:db"] = rel
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{Entity: "db2:db django:db", Action: "relate"})

	c.Assert(err, tc.ErrorIsNil)
	suspended := true
	results, err := s.api.PublishRelationChanges(c.Context(), params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{
			{
				Life:                    lifeValue,
				ForceCleanup:            &forceCleanup,
				Suspended:               &suspended,
				SuspendedReason:         suspendedReason,
				ApplicationOrOfferToken: "token-db2",
				RelationToken:           "token-db2:db django:db",
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId:   1,
					Settings: map[string]interface{}{"foo": "bar"},
				}},
				DepartedUnits: []int{2},
				Macaroons:     macaroon.Slice{mac.M()},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, tc.ErrorIsNil)
	expected := []testhelpers.StubCall{
		{FuncName: "GetRemoteEntity", Args: []interface{}{"token-db2:db django:db"}},
		{FuncName: "GetRemoteEntity", Args: []interface{}{"token-db2"}},
		{FuncName: "ApplicationOfferForUUID", Args: []interface{}{"f47ac10b-58cc-4372-a567-0e02b2c3d479"}},
		{FuncName: "KeyRelation", Args: []interface{}{"db2:db django:db"}},
	}
	if lifeValue == life.Alive {
		c.Assert(rel.status, tc.Equals, status.Suspending)
		if suspendedReason == "" {
			c.Assert(rel.message, tc.Equals, "suspending after update from remote model")
		} else {
			c.Assert(rel.message, tc.Equals, suspendedReason)
		}
	} else {
		c.Assert(rel.status, tc.Equals, status.Status(""))
		c.Assert(rel.message, tc.Equals, "")
	}
	s.st.CheckCalls(c, expected)
	if forceCleanup {
		ru1.CheckCalls(c, []testhelpers.StubCall{
			{FuncName: "LeaveScope", Args: []interface{}{}},
		})
		rel.CheckCalls(c, []testhelpers.StubCall{
			{FuncName: "Suspended", Args: []interface{}{}},
			{FuncName: "AllRemoteUnits", Args: []interface{}{"db2"}},
			{FuncName: "DestroyWithForce", Args: []interface{}{true}},
		})
	} else {
		ru1.CheckCalls(c, []testhelpers.StubCall{
			{FuncName: "InScope", Args: []interface{}{}},
			{FuncName: "EnterScope", Args: []interface{}{map[string]interface{}{"foo": "bar"}}},
		})
		if lifeValue == life.Alive {
			rel.CheckCalls(c, []testhelpers.StubCall{
				{FuncName: "Suspended", Args: []interface{}{}},
				{FuncName: "SetSuspended", Args: []interface{}{}},
				{FuncName: "SetStatus", Args: []interface{}{}},
				{FuncName: "Tag", Args: []interface{}{}},
				{FuncName: "RemoteUnit", Args: []interface{}{"db2/2"}},
				{FuncName: "RemoteUnit", Args: []interface{}{"db2/1"}},
			})
		} else {
			rel.CheckCalls(c, []testhelpers.StubCall{
				{FuncName: "Suspended", Args: []interface{}{}},
				{FuncName: "Destroy", Args: []interface{}{}},
				{FuncName: "Tag", Args: []interface{}{}},
				{FuncName: "RemoteUnit", Args: []interface{}{"db2/2"}},
				{FuncName: "RemoteUnit", Args: []interface{}{"db2/1"}},
			})
		}
	}
	ru2.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "LeaveScope", Args: []interface{}{}},
	})
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChanges(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertPublishRelationsChanges(c, life.Alive, "", false)
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChangesWithSuspendedReason(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertPublishRelationsChanges(c, life.Alive, "reason", false)
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChangesDyingWhileSuspended(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertPublishRelationsChanges(c, life.Dying, "", false)
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChangesDyingForceCleanup(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertPublishRelationsChanges(c, life.Dying, "", true)
}

func (s *crossmodelRelationsSuite) assertRegisterRemoteRelations(c *tc.C) {
	app := &mockApplication{}
	app.eps = []relation.Endpoint{{
		ApplicationName: "offeredapp",
		Relation:        charm.Relation{Name: "local"},
	}}
	s.st.applications["offeredapp"] = app
	s.st.offers = map[string]*crossmodel.ApplicationOffer{
		"f47ac10b-58cc-4372-a567-0e02b2c3d479": {
			OfferUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			OfferName:       "offered",
			ApplicationName: "offeredapp",
		}}
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("offer-uuid", "f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"f47ac10b-58cc-4372-a567-0e02b2c3d479", "consume"})

	c.Assert(err, tc.ErrorIsNil)
	results, err := s.api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{
		Relations: []params.RegisterRemoteRelationArg{{
			ApplicationToken:  "app-token",
			SourceModelTag:    coretesting.ModelTag.String(),
			RelationToken:     "rel-token",
			RemoteEndpoint:    params.RemoteEndpoint{Name: "remote"},
			OfferUUID:         "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			LocalEndpointName: "local",
			ConsumeVersion:    777,
			Macaroons:         macaroon.Slice{mac.M()},
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Check(result.Result.Token, tc.Equals, "token-f47ac10b-58cc-4372-a567-0e02b2c3d479")
	declared := checkers.InferDeclared(nil, macaroon.Slice{result.Result.Macaroon})
	c.Assert(declared, tc.DeepEquals, map[string]string{
		"source-model-uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"relation-key":      "offeredapp:local remote-apptoken:remote",
		"username":          "mary",
		"offer-uuid":        "f47ac10b-58cc-4372-a567-0e02b2c3d479",
	})
	cav := result.Result.Macaroon.Caveats()
	c.Check(cav, tc.HasLen, 5)
	c.Check(bytes.HasPrefix(cav[0].Id, []byte("time-before ")), tc.IsTrue)
	c.Check(cav[1].Id, tc.DeepEquals, []byte("declared source-model-uuid deadbeef-0bad-400d-8000-4b1d0d06f00d"))
	c.Check(cav[2].Id, tc.DeepEquals, []byte("declared offer-uuid f47ac10b-58cc-4372-a567-0e02b2c3d479"))
	c.Check(cav[3].Id, tc.DeepEquals, []byte("declared username mary"))
	c.Check(cav[4].Id, tc.DeepEquals, []byte("declared relation-key offeredapp:local remote-apptoken:remote"))

	expectedRemoteApp := s.st.remoteApplications["remote-apptoken"]
	expectedRemoteApp.Stub = testhelpers.Stub{} // don't care about api calls
	c.Check(expectedRemoteApp, tc.DeepEquals, &mockRemoteApplication{
		sourceModelUUID: coretesting.ModelTag.Id(), consumerproxy: true, consumeversion: 777})
	expectedRel := s.st.relations["offeredapp:local remote-apptoken:remote"]
	expectedRel.Stub = testhelpers.Stub{} // don't care about api calls
	c.Check(expectedRel, tc.DeepEquals, &mockRelation{id: 0, key: "offeredapp:local remote-apptoken:remote"})
	c.Check(s.st.remoteEntities, tc.HasLen, 2)
	c.Check(s.st.remoteEntities[names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")], tc.Equals, "token-f47ac10b-58cc-4372-a567-0e02b2c3d479")
	c.Check(s.st.remoteEntities[names.NewRelationTag("offeredapp:local remote-apptoken:remote")], tc.Equals, "rel-token")
	c.Assert(s.st.offerConnections, tc.HasLen, 1)
	offerConnection := s.st.offerConnections[0]
	c.Assert(offerConnection, tc.DeepEquals, &mockOfferConnection{
		sourcemodelUUID: coretesting.ModelTag.Id(),
		relationId:      0,
		relationKey:     "offeredapp:local remote-apptoken:remote",
		username:        "mary",
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
	})
}

func (s *crossmodelRelationsSuite) TestRegisterRemoteRelations(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertRegisterRemoteRelations(c)
}

func (s *crossmodelRelationsSuite) TestRegisterRemoteRelationsIdempotent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertRegisterRemoteRelations(c)
	s.assertRegisterRemoteRelations(c)
}

func (s *crossmodelRelationsSuite) TestPublishIngressNetworkChanges(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	rel := newMockRelation(1)
	rel.key = "db2:db django:db"
	s.st.relations["db2:db django:db"] = rel
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	modelConfig, err := config.New(config.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, tc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil)
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, tc.ErrorIsNil)
	results, err := s.api.PublishIngressNetworkChanges(c.Context(), params.IngressNetworksChanges{
		Changes: []params.IngressNetworksChangeEvent{
			{
				RelationToken: "token-db2:db django:db",
				Networks:      []string{"1.2.3.4/32"},
				Macaroons:     macaroon.Slice{mac.M()},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.st.ingressNetworks[rel.key], tc.DeepEquals, []string{"1.2.3.4/32"})
	s.st.CheckCalls(c, []testhelpers.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *crossmodelRelationsSuite) TestPublishIngressNetworkChangesRejected(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.relations["db2:db django:db"] = newMockRelation(1)
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	modelConfig, err := config.New(config.NoDefaults, coretesting.FakeConfig().Merge(
		coretesting.Attrs{
			config.SAASIngressAllowKey: "10.1.1.1/8",
		},
	))
	c.Assert(err, tc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil)
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, tc.ErrorIsNil)
	results, err := s.api.PublishIngressNetworkChanges(c.Context(), params.IngressNetworksChanges{
		Changes: []params.IngressNetworksChangeEvent{
			{
				RelationToken: "token-db2:db django:db",
				Networks:      []string{"1.2.3.4/32"},
				Macaroons:     macaroon.Slice{mac.M()},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta("subnet 1.2.3.4/32 not in firewall whitelist"))
	s.st.CheckCalls(c, []testhelpers.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *crossmodelRelationsSuite) TestWatchEgressAddressesForRelations(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, tc.ErrorIsNil)
	args := params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{
			{
				Token:     "token-mysql:db django:db",
				Macaroons: macaroon.Slice{mac.M()},
			},
			{
				Token:     "token-db2:db django:db",
				Macaroons: macaroon.Slice{mac.M()},
			},
			{
				Token:     "token-postgresql:db django:db",
				Macaroons: macaroon.Slice{mac.M()},
			},
		},
	}
	results, err := s.api.WatchEgressAddressesForRelations(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error.ErrorCode(), tc.Equals, params.CodeNotFound)
	c.Assert(results.Results[1].Error, tc.IsNil)
	c.Assert(results.Results[2].Error.ErrorCode(), tc.Equals, params.CodeNotFound)
	c.Assert(s.watchedRelations, tc.DeepEquals, params.Entities{
		Entities: []params.Entity{{Tag: "relation-db2.db#django.db"}}},
	)
	s.st.CheckCalls(c, []testhelpers.StubCall{
		{"GetRemoteEntity", []interface{}{"token-mysql:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-postgresql:db django:db"}},
	})
	// TODO(wallyworld) - add mre tests when implementation finished
}

func (s *crossmodelRelationsSuite) TestWatchRelationsStatus(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	rel := newMockRelation(1)
	s.st.relations["db2:db django:db"] = rel
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, tc.ErrorIsNil)
	args := params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{
			{
				Token:     "token-mysql:db django:db",
				Macaroons: macaroon.Slice{mac.M()},
			},
			{
				Token:     "token-db2:db django:db",
				Macaroons: macaroon.Slice{mac.M()},
			},
		},
	}
	results, err := s.api.WatchRelationsSuspendedStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error.ErrorCode(), tc.Equals, params.CodeNotFound)
	c.Assert(results.Results[1].Error, tc.IsNil)
	c.Assert(s.watchedRelations, tc.DeepEquals, params.Entities{
		Entities: []params.Entity{{Tag: "relation-db2.db#django.db"}}},
	)
	s.st.CheckCalls(c, []testhelpers.StubCall{
		{"GetRemoteEntity", []interface{}{"token-mysql:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *crossmodelRelationsSuite) TestWatchRelationsStatusRelationNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, tc.ErrorIsNil)
	args := params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{
			{
				Token:     "token-db2:db django:db",
				Macaroons: macaroon.Slice{mac.M()},
			},
		},
	}

	// First check that when not migrating, we see the relation as dead.
	results, err := s.api.WatchRelationsSuspendedStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].Changes[0].Life, tc.Equals, life.Dead)
	s.st.CheckCalls(c, []testhelpers.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"IsMigrationActive", []interface{}{}},
	})
	s.st.ResetCalls()

	// Now indicate that a migration is active
	// and ensure that the error flows to us.
	s.st.migrationActive = true
	results, err = s.api.WatchRelationsSuspendedStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
	s.st.CheckCalls(c, []testhelpers.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"IsMigrationActive", []interface{}{}},
	})
}

func (s *crossmodelRelationsSuite) TestWatchOfferStatus(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.st.offers["f47ac10b-58cc-4372-a567-0e02b2c3d479"] = &crossmodel.ApplicationOffer{
		OfferName: "hosted-mysql", OfferUUID: "f47ac10b-58cc-4372-a567-0e02b2c3d479", ApplicationName: "mysql"}
	s.st.remoteEntities[names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")] = "token-hosted-mysql"
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("offer-uuid", "f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"f47ac10b-58cc-4372-a567-0e02b2c3d479", "consume"})

	c.Assert(err, tc.ErrorIsNil)
	args := params.OfferArgs{
		Args: []params.OfferArg{
			{
				OfferUUID: "db2-uuid",
				Macaroons: macaroon.Slice{mac.M()},
			},
			{
				OfferUUID: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
				Macaroons: macaroon.Slice{mac.M()},
			},
			{
				OfferUUID: "postgresql-uuid",
				Macaroons: macaroon.Slice{mac.M()},
			},
		},
	}

	s.statusService.EXPECT().GetApplicationDisplayStatus(gomock.Any(), "mysql").Return(status.StatusInfo{
		Status: status.Waiting,
	}, nil)

	results, err := s.api.WatchOfferStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error.ErrorCode(), tc.Equals, params.CodeUnauthorized)
	c.Assert(results.Results[1].Error, tc.IsNil)
	// Check against a non-terminating status to show that the status is
	// coming from the application.
	c.Assert(results.Results[1].Changes, tc.DeepEquals, []params.OfferStatusChange{{
		OfferName: "mysql",
		Status:    params.EntityStatus{Status: status.Waiting},
	}})
	c.Assert(results.Results[2].Error.ErrorCode(), tc.Equals, params.CodeUnauthorized)
	c.Assert(s.watchedOffers, tc.DeepEquals, []string{"f47ac10b-58cc-4372-a567-0e02b2c3d479"})
	s.st.CheckCalls(c, []testhelpers.StubCall{
		{"IsMigrationActive", nil},
		{"ApplicationOfferForUUID", []interface{}{"f47ac10b-58cc-4372-a567-0e02b2c3d479"}},
	})
}

func (s *crossmodelRelationsSuite) TestPublishChangesWithApplicationSettingsRemoteEntityOfferTag(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.remoteEntities[names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")] = "token-db2"
	s.st.offers["f47ac10b-58cc-4372-a567-0e02b2c3d479"] = &crossmodel.ApplicationOffer{
		OfferName: "db2-offer", ApplicationName: "db2"}
	rel := newMockRelation(1)
	ru1 := newMockRelationUnit()
	ru2 := newMockRelationUnit()
	rel.units["db2/1"] = ru1
	rel.units["db2/2"] = ru2
	s.st.relations["db2:db django:db"] = rel
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, tc.ErrorIsNil)
	results, err := s.api.PublishRelationChanges(c.Context(), params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{
			{
				Life:                    life.Alive,
				ApplicationOrOfferToken: "token-db2",
				RelationToken:           "token-db2:db django:db",
				ApplicationSettings: map[string]interface{}{
					"slaughterhouse": "the-tongue",
				},
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId:   1,
					Settings: map[string]interface{}{"foo": "bar"},
				}},
				DepartedUnits: []int{2},
				Macaroons:     macaroon.Slice{mac.M()},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, tc.ErrorIsNil)
	expected := []testhelpers.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2"}},
		{"ApplicationOfferForUUID", []interface{}{"f47ac10b-58cc-4372-a567-0e02b2c3d479"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	}
	s.st.CheckCalls(c, expected)
	ru1.CheckCalls(c, []testhelpers.StubCall{
		{"InScope", []interface{}{}},
		{"EnterScope", []interface{}{map[string]interface{}{"foo": "bar"}}},
	})
	ru2.CheckCalls(c, []testhelpers.StubCall{
		{"LeaveScope", []interface{}{}},
	})
	rel.CheckCallNames(c, "Suspended", "ReplaceApplicationSettings", "Tag", "RemoteUnit", "RemoteUnit")
	rel.CheckCall(c, 1, "ReplaceApplicationSettings", "db2", map[string]interface{}{
		"slaughterhouse": "the-tongue",
	})
}

func (s *crossmodelRelationsSuite) TestPublishChangesWithApplicationSettingsRemoteEntityApplicationTag(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	s.st.offers["f47ac10b-58cc-4372-a567-0e02b2c3d479"] = &crossmodel.ApplicationOffer{
		OfferName: "db2-offer", ApplicationName: "db2"}
	rel := newMockRelation(1)
	ru1 := newMockRelationUnit()
	ru2 := newMockRelationUnit()
	rel.units["db2/1"] = ru1
	rel.units["db2/2"] = ru2
	s.st.relations["db2:db django:db"] = rel
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, tc.ErrorIsNil)
	results, err := s.api.PublishRelationChanges(c.Context(), params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{
			{
				Life:                    life.Alive,
				ApplicationOrOfferToken: "token-db2",
				RelationToken:           "token-db2:db django:db",
				ApplicationSettings: map[string]interface{}{
					"slaughterhouse": "the-tongue",
				},
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId:   1,
					Settings: map[string]interface{}{"foo": "bar"},
				}},
				DepartedUnits: []int{2},
				Macaroons:     macaroon.Slice{mac.M()},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, tc.ErrorIsNil)
	expected := []testhelpers.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	}
	s.st.CheckCalls(c, expected)
	ru1.CheckCalls(c, []testhelpers.StubCall{
		{"InScope", []interface{}{}},
		{"EnterScope", []interface{}{map[string]interface{}{"foo": "bar"}}},
	})
	ru2.CheckCalls(c, []testhelpers.StubCall{
		{"LeaveScope", []interface{}{}},
	})
	rel.CheckCallNames(c, "Suspended", "ReplaceApplicationSettings", "Tag", "RemoteUnit", "RemoteUnit")
	rel.CheckCall(c, 1, "ReplaceApplicationSettings", "db2", map[string]interface{}{
		"slaughterhouse": "the-tongue",
	})
}

func ptr[T any](v T) *T {
	return &v
}

func (s *crossmodelRelationsSuite) TestResumeRelationPermissionCheck(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.authorizer.AdminTag = names.NewUserTag("fred")
	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	rel := newMockRelation(1)
	rel.suspended = true
	ru1 := newMockRelationUnit()
	ru2 := newMockRelationUnit()
	rel.units["db2/1"] = ru1
	rel.units["db2/2"] = ru2
	s.st.relations["db2:db django:db"] = rel
	s.st.offers["f47ac10b-58cc-4372-a567-0e02b2c3d479"] = &crossmodel.ApplicationOffer{ApplicationName: "db2"}
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		username:        "mary",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, tc.ErrorIsNil)
	results, err := s.api.PublishRelationChanges(c.Context(), params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{
			{
				Suspended:               ptr(false),
				Life:                    life.Alive,
				ApplicationOrOfferToken: "token-db2",
				RelationToken:           "token-db2:db django:db",
				Macaroons:               macaroon.Slice{mac.M()},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *crossmodelRelationsSuite) TestWatchRelationChanges(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	s.st.applications["django"] = &mockApplication{}
	s.st.remoteEntities[names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")] = "token-hosted-db2"
	rel := newMockRelation(1)
	ru1 := newMockRelationUnit()
	ru2 := newMockRelationUnit()

	ru1.settings["che-fu"] = "fade away"

	rel.endpoints = append(rel.endpoints,
		relation.Endpoint{ApplicationName: "db2"},
		relation.Endpoint{ApplicationName: "django"},
	)
	rel.units["django/1"] = ru1
	rel.units["django/2"] = ru2

	w := &mockUnitsWatcher{
		mockWatcher: &mockWatcher{
			stopped: make(chan struct{}),
		},
		changes: make(chan watcher.RelationUnitsChange, 1),
	}
	w.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/1": {Version: 100},
		},
		AppChanged: map[string]int64{
			"django": 123,
		},
		Departed: []string{"django/0", "django/2"},
	}
	rel.watchers["django"] = w

	rel.appSettings["django"] = map[string]interface{}{
		"majoribanks": "mt victoria",
	}

	s.st.relations["db2:db django:db"] = rel
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.offerUUIDs["db2:db django:db"] = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.api.WatchRelationChanges(c.Context(), params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{{
			Token:     "token-db2:db django:db",
			Macaroons: macaroon.Slice{mac.M()},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	uc := 666
	c.Assert(result, tc.DeepEquals, params.RemoteRelationWatchResults{
		Results: []params.RemoteRelationWatchResult{{
			RemoteRelationWatcherId: "1",
			Changes: params.RemoteRelationChangeEvent{
				RelationToken:           "token-db2:db django:db",
				ApplicationOrOfferToken: "token-hosted-db2",
				Macaroons:               nil,
				UnitCount:               &uc,
				ApplicationSettings: map[string]interface{}{
					"majoribanks": "mt victoria",
				},
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId: 1,
					Settings: map[string]interface{}{
						"che-fu": "fade away",
					},
				}},
				DepartedUnits: []int{0, 2},
			},
		}},
	})

	c.Assert(s.resources.Count(), tc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	outw, ok := resource.(*commoncrossmodel.WrappedUnitsWatcher)
	c.Assert(ok, tc.Equals, true)
	c.Assert(outw.RelationToken, tc.Equals, "token-db2:db django:db")
	c.Assert(outw.ApplicationOrOfferToken, tc.Equals, "token-hosted-db2")

	// TODO(babbageclunk): add locking around updating mock
	// relation/relunit settings.
	rel.appSettings["django"]["majoribanks"] = "roxburgh"
	change := watcher.RelationUnitsChange{
		AppChanged: map[string]int64{"django": 124},
	}
	select {
	case w.changes <- change:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out sending event to internal watcher")
	}

	select {
	case event := <-outw.Changes():
		c.Assert(event, tc.DeepEquals, params.RelationUnitsChange{
			AppChanged: map[string]int64{"django": 124},
		})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out receiving change event")
	}
}

func (s *crossmodelRelationsSuite) TestWatchConsumedSecretsChanges(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.secretService.secrets["9m4e2mr0ui3e8a215n4g"] = coresecrets.SecretMetadata{LatestRevision: 666}
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	s.st.remoteEntities[names.NewApplicationTag("postgresql")] = "token-postgresql"
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-rel-db2"
	s.st.remoteEntities[names.NewRelationTag("postgresql:db django:db")] = "token-rel-postgresql"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID: "token-rel-db2-uuid",
	}
	s.st.offerConnectionsByKey["postgresql:db django:db"] = &mockOfferConnection{
		offerUUID: "token-rel-postgresql-uuid",
	}

	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("offer-uuid", "token-rel-db2-uuid"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"token-rel-db2-uuid", "consume"})

	c.Assert(err, tc.ErrorIsNil)
	args := params.WatchRemoteSecretChangesArgs{
		Args: []params.WatchRemoteSecretChangesArg{
			{
				ApplicationToken: "token-db2",
				RelationToken:    "token-rel-db2",
				Macaroons:        macaroon.Slice{mac.M()},
			},
			{
				ApplicationToken: "token-mysql",
				RelationToken:    "token-rel-mysql",
				Macaroons:        macaroon.Slice{mac.M()},
			},
			{
				ApplicationToken: "token-postgresql",
				RelationToken:    "token-rel-postgresql",
				Macaroons:        macaroon.Slice{mac.M()},
			},
		},
	}
	results, err := s.api.WatchConsumedSecretsChanges(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].Changes, tc.DeepEquals, []params.SecretRevisionChange{{
		URI:            "secret:9m4e2mr0ui3e8a215n4g",
		LatestRevision: 666,
	}})
	c.Assert(results.Results[1].Error.ErrorCode(), tc.Equals, params.CodeUnauthorized)
	c.Assert(results.Results[2].Error.ErrorCode(), tc.Equals, params.CodeUnauthorized)
	c.Assert(s.watchedSecretConsumers, tc.DeepEquals, []string{"db2"})
}
