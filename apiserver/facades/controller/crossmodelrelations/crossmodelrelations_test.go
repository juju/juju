// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"bytes"
	"context"
	"regexp"
	"sync"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
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
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/relation"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(&crossmodelRelationsSuite{})

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

func (s *crossmodelRelationsSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.statusService = NewMockStatusService(ctrl)

	return ctrl
}

func (s *crossmodelRelationsSuite) setupAPI(c *gc.C) {
	s.bakery = &mockBakeryService{}
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState()
	fw := &mockFirewallState{}
	egressAddressWatcher := func(_ facade.Resources, fws firewall.State, modelConfigService firewall.ModelConfigService, relations params.Entities) (params.StringsWatchResults, error) {
		c.Assert(fw, gc.Equals, fws)
		s.watchedRelations = relations
		return params.StringsWatchResults{Results: make([]params.StringsWatchResult, len(relations.Entities))}, nil
	}
	relationStatusWatcher := func(st crossmodelrelations.CrossModelRelationsState, tag names.RelationTag) (state.StringsWatcher, error) {
		c.Assert(s.st, gc.Equals, st)
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
		c.Assert(s.st, gc.Equals, st)
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
	c.Assert(err, jc.ErrorIsNil)
	s.secretService = newMockSecretService()
	api, err := crossmodelrelations.NewCrossModelRelationsAPI(
		model.UUID(coretesting.ModelTag.Id()),
		s.st, fw, s.resources, s.authorizer,
		authContext, s.secretService, s.modelConfigService, s.statusService, egressAddressWatcher, relationStatusWatcher,
		offerStatusWatcher, consumedSecretsWatcher,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *crossmodelRelationsSuite) assertPublishRelationsChanges(c *gc.C, lifeValue life.Value, suspendedReason string, forceCleanup bool) {
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
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	suspended := true
	results, err := s.api.PublishRelationChanges(context.Background(), params.RemoteRelationsChanges{
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
	c.Assert(err, jc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, jc.ErrorIsNil)
	expected := []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2"}},
		{"ApplicationOfferForUUID", []interface{}{"f47ac10b-58cc-4372-a567-0e02b2c3d479"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	}
	if lifeValue == life.Alive {
		c.Assert(rel.status, gc.Equals, status.Suspending)
		if suspendedReason == "" {
			c.Assert(rel.message, gc.Equals, "suspending after update from remote model")
		} else {
			c.Assert(rel.message, gc.Equals, suspendedReason)
		}
	} else {
		c.Assert(rel.status, gc.Equals, status.Status(""))
		c.Assert(rel.message, gc.Equals, "")
	}
	s.st.CheckCalls(c, expected)
	if forceCleanup {
		ru1.CheckCalls(c, []testing.StubCall{
			{"LeaveScope", []interface{}{}},
		})
		rel.CheckCalls(c, []testing.StubCall{
			{"Suspended", []interface{}{}},
			{"AllRemoteUnits", []interface{}{"db2"}},
			{"DestroyWithForce", []interface{}{true}},
		})
	} else {
		ru1.CheckCalls(c, []testing.StubCall{
			{"InScope", []interface{}{}},
			{"EnterScope", []interface{}{map[string]interface{}{"foo": "bar"}}},
		})
		if lifeValue == life.Alive {
			rel.CheckCalls(c, []testing.StubCall{
				{"Suspended", []interface{}{}},
				{"SetSuspended", []interface{}{}},
				{"SetStatus", []interface{}{}},
				{"Tag", []interface{}{}},
				{"RemoteUnit", []interface{}{"db2/2"}},
				{"RemoteUnit", []interface{}{"db2/1"}},
			})
		} else {
			rel.CheckCalls(c, []testing.StubCall{
				{"Suspended", []interface{}{}},
				{"Destroy", []interface{}{}},
				{"Tag", []interface{}{}},
				{"RemoteUnit", []interface{}{"db2/2"}},
				{"RemoteUnit", []interface{}{"db2/1"}},
			})
		}
	}
	ru2.CheckCalls(c, []testing.StubCall{
		{"LeaveScope", []interface{}{}},
	})
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChanges(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertPublishRelationsChanges(c, life.Alive, "", false)
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChangesWithSuspendedReason(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertPublishRelationsChanges(c, life.Alive, "reason", false)
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChangesDyingWhileSuspended(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertPublishRelationsChanges(c, life.Dying, "", false)
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChangesDyingForceCleanup(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertPublishRelationsChanges(c, life.Dying, "", true)
}

func (s *crossmodelRelationsSuite) assertRegisterRemoteRelations(c *gc.C) {
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
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("offer-uuid", "f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"f47ac10b-58cc-4372-a567-0e02b2c3d479", "consume"})

	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.RegisterRemoteRelations(context.Background(), params.RegisterRemoteRelationArgs{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)
	c.Check(result.Result.Token, gc.Equals, "token-f47ac10b-58cc-4372-a567-0e02b2c3d479")
	declared := checkers.InferDeclared(nil, macaroon.Slice{result.Result.Macaroon})
	c.Assert(declared, jc.DeepEquals, map[string]string{
		"source-model-uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"relation-key":      "offeredapp:local remote-apptoken:remote",
		"username":          "mary",
		"offer-uuid":        "f47ac10b-58cc-4372-a567-0e02b2c3d479",
	})
	cav := result.Result.Macaroon.Caveats()
	c.Check(cav, gc.HasLen, 5)
	c.Check(bytes.HasPrefix(cav[0].Id, []byte("time-before ")), jc.IsTrue)
	c.Check(cav[1].Id, jc.DeepEquals, []byte("declared source-model-uuid deadbeef-0bad-400d-8000-4b1d0d06f00d"))
	c.Check(cav[2].Id, jc.DeepEquals, []byte("declared offer-uuid f47ac10b-58cc-4372-a567-0e02b2c3d479"))
	c.Check(cav[3].Id, jc.DeepEquals, []byte("declared username mary"))
	c.Check(cav[4].Id, jc.DeepEquals, []byte("declared relation-key offeredapp:local remote-apptoken:remote"))

	expectedRemoteApp := s.st.remoteApplications["remote-apptoken"]
	expectedRemoteApp.Stub = testing.Stub{} // don't care about api calls
	c.Check(expectedRemoteApp, jc.DeepEquals, &mockRemoteApplication{
		sourceModelUUID: coretesting.ModelTag.Id(), consumerproxy: true, consumeversion: 777})
	expectedRel := s.st.relations["offeredapp:local remote-apptoken:remote"]
	expectedRel.Stub = testing.Stub{} // don't care about api calls
	c.Check(expectedRel, jc.DeepEquals, &mockRelation{id: 0, key: "offeredapp:local remote-apptoken:remote"})
	c.Check(s.st.remoteEntities, gc.HasLen, 2)
	c.Check(s.st.remoteEntities[names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")], gc.Equals, "token-f47ac10b-58cc-4372-a567-0e02b2c3d479")
	c.Check(s.st.remoteEntities[names.NewRelationTag("offeredapp:local remote-apptoken:remote")], gc.Equals, "rel-token")
	c.Assert(s.st.offerConnections, gc.HasLen, 1)
	offerConnection := s.st.offerConnections[0]
	c.Assert(offerConnection, jc.DeepEquals, &mockOfferConnection{
		sourcemodelUUID: coretesting.ModelTag.Id(),
		relationId:      0,
		relationKey:     "offeredapp:local remote-apptoken:remote",
		username:        "mary",
		offerUUID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
	})
}

func (s *crossmodelRelationsSuite) TestRegisterRemoteRelations(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertRegisterRemoteRelations(c)
}

func (s *crossmodelRelationsSuite) TestRegisterRemoteRelationsIdempotent(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.assertRegisterRemoteRelations(c)
	s.assertRegisterRemoteRelations(c)
}

func (s *crossmodelRelationsSuite) TestPublishIngressNetworkChanges(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil)
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.PublishIngressNetworkChanges(context.Background(), params.IngressNetworksChanges{
		Changes: []params.IngressNetworksChangeEvent{
			{
				RelationToken: "token-db2:db django:db",
				Networks:      []string{"1.2.3.4/32"},
				Macaroons:     macaroon.Slice{mac.M()},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.st.ingressNetworks[rel.key], jc.DeepEquals, []string{"1.2.3.4/32"})
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *crossmodelRelationsSuite) TestPublishIngressNetworkChangesRejected(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil)
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.PublishIngressNetworkChanges(context.Background(), params.IngressNetworksChanges{
		Changes: []params.IngressNetworksChangeEvent{
			{
				RelationToken: "token-db2:db django:db",
				Networks:      []string{"1.2.3.4/32"},
				Macaroons:     macaroon.Slice{mac.M()},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta("subnet 1.2.3.4/32 not in firewall whitelist"))
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *crossmodelRelationsSuite) TestWatchEgressAddressesForRelations(c *gc.C) {
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
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
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
	results, err := s.api.WatchEgressAddressesForRelations(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error.ErrorCode(), gc.Equals, params.CodeNotFound)
	c.Assert(results.Results[1].Error, gc.IsNil)
	c.Assert(results.Results[2].Error.ErrorCode(), gc.Equals, params.CodeNotFound)
	c.Assert(s.watchedRelations, jc.DeepEquals, params.Entities{
		Entities: []params.Entity{{Tag: "relation-db2.db#django.db"}}},
	)
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-mysql:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-postgresql:db django:db"}},
	})
	// TODO(wallyworld) - add mre tests when implementation finished
}

func (s *crossmodelRelationsSuite) TestWatchRelationsStatus(c *gc.C) {
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
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
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
	results, err := s.api.WatchRelationsSuspendedStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error.ErrorCode(), gc.Equals, params.CodeNotFound)
	c.Assert(results.Results[1].Error, gc.IsNil)
	c.Assert(s.watchedRelations, jc.DeepEquals, params.Entities{
		Entities: []params.Entity{{Tag: "relation-db2.db#django.db"}}},
	)
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-mysql:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *crossmodelRelationsSuite) TestWatchRelationsStatusRelationNotFound(c *gc.C) {
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
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	args := params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{
			{
				Token:     "token-db2:db django:db",
				Macaroons: macaroon.Slice{mac.M()},
			},
		},
	}

	// First check that when not migrating, we see the relation as dead.
	results, err := s.api.WatchRelationsSuspendedStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Changes[0].Life, gc.Equals, life.Dead)
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"IsMigrationActive", []interface{}{}},
	})
	s.st.ResetCalls()

	// Now indicate that a migration is active
	// and ensure that the error flows to us.
	s.st.migrationActive = true
	results, err = s.api.WatchRelationsSuspendedStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error.Code, gc.Equals, params.CodeNotFound)
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"IsMigrationActive", []interface{}{}},
	})
}

func (s *crossmodelRelationsSuite) TestWatchOfferStatus(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.st.offers["f47ac10b-58cc-4372-a567-0e02b2c3d479"] = &crossmodel.ApplicationOffer{
		OfferName: "hosted-mysql", OfferUUID: "f47ac10b-58cc-4372-a567-0e02b2c3d479", ApplicationName: "mysql"}
	s.st.remoteEntities[names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")] = "token-hosted-mysql"
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("offer-uuid", "f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"f47ac10b-58cc-4372-a567-0e02b2c3d479", "consume"})

	c.Assert(err, jc.ErrorIsNil)
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

	s.statusService.EXPECT().GetApplicationDisplayStatus(gomock.Any(), "mysql").Return(&status.StatusInfo{
		Status: status.Waiting,
	}, nil)

	results, err := s.api.WatchOfferStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error.ErrorCode(), gc.Equals, params.CodeUnauthorized)
	c.Assert(results.Results[1].Error, gc.IsNil)
	// Check against a non-terminating status to show that the status is
	// coming from the application.
	c.Assert(results.Results[1].Changes, jc.DeepEquals, []params.OfferStatusChange{{
		OfferName: "mysql",
		Status:    params.EntityStatus{Status: status.Waiting},
	}})
	c.Assert(results.Results[2].Error.ErrorCode(), gc.Equals, params.CodeUnauthorized)
	c.Assert(s.watchedOffers, jc.DeepEquals, []string{"f47ac10b-58cc-4372-a567-0e02b2c3d479"})
	s.st.CheckCalls(c, []testing.StubCall{
		{"IsMigrationActive", nil},
		{"ApplicationOfferForUUID", []interface{}{"f47ac10b-58cc-4372-a567-0e02b2c3d479"}},
	})
}

func (s *crossmodelRelationsSuite) TestPublishChangesWithApplicationSettingsRemoteEntityOfferTag(c *gc.C) {
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
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.PublishRelationChanges(context.Background(), params.RemoteRelationsChanges{
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
	c.Assert(err, jc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, jc.ErrorIsNil)
	expected := []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2"}},
		{"ApplicationOfferForUUID", []interface{}{"f47ac10b-58cc-4372-a567-0e02b2c3d479"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	}
	s.st.CheckCalls(c, expected)
	ru1.CheckCalls(c, []testing.StubCall{
		{"InScope", []interface{}{}},
		{"EnterScope", []interface{}{map[string]interface{}{"foo": "bar"}}},
	})
	ru2.CheckCalls(c, []testing.StubCall{
		{"LeaveScope", []interface{}{}},
	})
	rel.CheckCallNames(c, "Suspended", "ReplaceApplicationSettings", "Tag", "RemoteUnit", "RemoteUnit")
	rel.CheckCall(c, 1, "ReplaceApplicationSettings", "db2", map[string]interface{}{
		"slaughterhouse": "the-tongue",
	})
}

func (s *crossmodelRelationsSuite) TestPublishChangesWithApplicationSettingsRemoteEntityApplicationTag(c *gc.C) {
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
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.PublishRelationChanges(context.Background(), params.RemoteRelationsChanges{
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
	c.Assert(err, jc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, jc.ErrorIsNil)
	expected := []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	}
	s.st.CheckCalls(c, expected)
	ru1.CheckCalls(c, []testing.StubCall{
		{"InScope", []interface{}{}},
		{"EnterScope", []interface{}{map[string]interface{}{"foo": "bar"}}},
	})
	ru2.CheckCalls(c, []testing.StubCall{
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

func (s *crossmodelRelationsSuite) TestResumeRelationPermissionCheck(c *gc.C) {
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
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.PublishRelationChanges(context.Background(), params.RemoteRelationsChanges{
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
	c.Assert(err, jc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *crossmodelRelationsSuite) TestWatchRelationChanges(c *gc.C) {
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
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.WatchRelationChanges(context.Background(), params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{{
			Token:     "token-db2:db django:db",
			Macaroons: macaroon.Slice{mac.M()},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	uc := 666
	c.Assert(result, gc.DeepEquals, params.RemoteRelationWatchResults{
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

	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	outw, ok := resource.(*commoncrossmodel.WrappedUnitsWatcher)
	c.Assert(ok, gc.Equals, true)
	c.Assert(outw.RelationToken, gc.Equals, "token-db2:db django:db")
	c.Assert(outw.ApplicationOrOfferToken, gc.Equals, "token-hosted-db2")

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
		c.Assert(event, gc.DeepEquals, params.RelationUnitsChange{
			AppChanged: map[string]int64{"django": 124},
		})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out receiving change event")
	}
}

func (s *crossmodelRelationsSuite) TestWatchConsumedSecretsChanges(c *gc.C) {
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
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("offer-uuid", "token-rel-db2-uuid"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"token-rel-db2-uuid", "consume"})

	c.Assert(err, jc.ErrorIsNil)
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
	results, err := s.api.WatchConsumedSecretsChanges(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Changes, jc.DeepEquals, []params.SecretRevisionChange{{
		URI:            "secret:9m4e2mr0ui3e8a215n4g",
		LatestRevision: 666,
	}})
	c.Assert(results.Results[1].Error.ErrorCode(), gc.Equals, params.CodeUnauthorized)
	c.Assert(results.Results[2].Error.ErrorCode(), gc.Equals, params.CodeUnauthorized)
	c.Assert(s.watchedSecretConsumers, jc.DeepEquals, []string{"db2"})
}
