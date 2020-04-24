// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"bytes"
	"context"
	"regexp"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	corefirewall "github.com/juju/juju/core/firewall"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&crossmodelRelationsSuite{})

type crossmodelRelationsSuite struct {
	coretesting.BaseSuite

	resources     *common.Resources
	authorizer    *apiservertesting.FakeAuthorizer
	st            *mockState
	mockStatePool *mockStatePool
	bakery        *mockBakeryService
	authContext   *commoncrossmodel.AuthContext
	api           *crossmodelrelations.CrossModelRelationsAPI

	watchedRelations params.Entities
	watchedOffers    []string
}

func (s *crossmodelRelationsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.bakery = &mockBakeryService{}
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState()
	s.mockStatePool = &mockStatePool{map[string]commoncrossmodel.Backend{coretesting.ModelTag.Id(): s.st}}
	fw := &mockFirewallState{}
	egressAddressWatcher := func(_ facade.Resources, fws firewall.State, relations params.Entities) (params.StringsWatchResults, error) {
		c.Assert(fw, gc.Equals, fws)
		s.watchedRelations = relations
		return params.StringsWatchResults{Results: make([]params.StringsWatchResult, len(relations.Entities))}, nil
	}
	relationStatusWatcher := func(st crossmodelrelations.CrossModelRelationsState, tag names.RelationTag) (state.StringsWatcher, error) {
		c.Assert(s.st, gc.Equals, st)
		s.watchedRelations = params.Entities{Entities: []params.Entity{{Tag: tag.String()}}}
		w := &mockRelationStatusWatcher{changes: make(chan []string, 1)}
		w.changes <- []string{"db2:db django:db"}
		return w, nil
	}
	offerStatusWatcher := func(st crossmodelrelations.CrossModelRelationsState, offerUUID string) (crossmodelrelations.OfferWatcher, error) {
		c.Assert(s.st, gc.Equals, st)
		s.watchedOffers = []string{offerUUID}
		w := &mockOfferStatusWatcher{offerUUID: offerUUID, changes: make(chan struct{}, 1)}
		w.changes <- struct{}{}
		return w, nil
	}
	var err error
	thirdPartyKey := bakery.MustGenerateKey()
	s.authContext, err = commoncrossmodel.NewAuthContext(s.mockStatePool, thirdPartyKey, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	api, err := crossmodelrelations.NewCrossModelRelationsAPI(
		s.st, fw, s.resources, s.authorizer, s.authContext, egressAddressWatcher, relationStatusWatcher, offerStatusWatcher)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *crossmodelRelationsSuite) assertPublishRelationsChanges(c *gc.C, lifeValue life.Value, suspendedReason string, forceCleanup bool) {
	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	rel := newMockRelation(1)
	ru1 := newMockRelationUnit()
	ru2 := newMockRelationUnit()
	rel.units["db2/1"] = ru1
	rel.units["db2/2"] = ru2
	s.st.relations["db2:db django:db"] = rel
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "hosted-db2-uuid",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	suspended := true
	results, err := s.api.PublishRelationChanges(params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{
			{
				Life:             lifeValue,
				ForceCleanup:     &forceCleanup,
				Suspended:        &suspended,
				SuspendedReason:  suspendedReason,
				ApplicationToken: "token-db2",
				RelationToken:    "token-db2:db django:db",
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
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2"}},
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
		expected = append(expected, testing.StubCall{
			"RemoteApplication", []interface{}{"db2"},
		})
	}
	s.st.CheckCalls(c, expected)
	if forceCleanup {
		ru1.CheckCalls(c, []testing.StubCall{
			{"LeaveScope", []interface{}{}},
		})
	} else {
		ru1.CheckCalls(c, []testing.StubCall{
			{"InScope", []interface{}{}},
			{"EnterScope", []interface{}{map[string]interface{}{"foo": "bar"}}},
		})
	}
	ru2.CheckCalls(c, []testing.StubCall{
		{"LeaveScope", []interface{}{}},
	})
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChanges(c *gc.C) {
	s.assertPublishRelationsChanges(c, life.Alive, "", false)
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChangesWithSuspendedReason(c *gc.C) {
	s.assertPublishRelationsChanges(c, life.Alive, "reason", false)
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChangesDyingWhileSuspended(c *gc.C) {
	s.assertPublishRelationsChanges(c, life.Dying, "", false)
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChangesDyingForceCleanup(c *gc.C) {
	s.assertPublishRelationsChanges(c, life.Dying, "", true)
}

func (s *crossmodelRelationsSuite) assertRegisterRemoteRelations(c *gc.C) {
	app := &mockApplication{}
	app.eps = []state.Endpoint{{
		ApplicationName: "offeredapp",
		Relation:        charm.Relation{Name: "local"},
	}}
	s.st.applications["offeredapp"] = app
	s.st.offers = map[string]*crossmodel.ApplicationOffer{
		"offer-uuid": {
			OfferUUID:       "offer-uuid",
			OfferName:       "offered",
			ApplicationName: "offeredapp",
		}}
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "offer-uuid",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("offer-uuid", "offer-uuid"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"offer-uuid", "consume"})

	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.RegisterRemoteRelations(params.RegisterRemoteRelationArgs{
		Relations: []params.RegisterRemoteRelationArg{{
			ApplicationToken:  "app-token",
			SourceModelTag:    coretesting.ModelTag.String(),
			RelationToken:     "rel-token",
			RemoteEndpoint:    params.RemoteEndpoint{Name: "remote"},
			OfferUUID:         "offer-uuid",
			LocalEndpointName: "local",
			Macaroons:         macaroon.Slice{mac.M()},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)
	c.Check(result.Result.Token, gc.Equals, "token-offered")
	declared := checkers.InferDeclared(nil, macaroon.Slice{result.Result.Macaroon})
	c.Assert(declared, jc.DeepEquals, map[string]string{
		"source-model-uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"relation-key":      "offeredapp:local remote-apptoken:remote",
		"username":          "mary",
		"offer-uuid":        "offer-uuid",
	})
	cav := result.Result.Macaroon.Caveats()
	c.Check(cav, gc.HasLen, 5)
	c.Check(bytes.HasPrefix(cav[0].Id, []byte("time-before ")), jc.IsTrue)
	c.Check(cav[1].Id, jc.DeepEquals, []byte("declared source-model-uuid deadbeef-0bad-400d-8000-4b1d0d06f00d"))
	c.Check(cav[2].Id, jc.DeepEquals, []byte("declared offer-uuid offer-uuid"))
	c.Check(cav[3].Id, jc.DeepEquals, []byte("declared username mary"))
	c.Check(cav[4].Id, jc.DeepEquals, []byte("declared relation-key offeredapp:local remote-apptoken:remote"))

	expectedRemoteApp := s.st.remoteApplications["remote-apptoken"]
	expectedRemoteApp.Stub = testing.Stub{} // don't care about api calls
	c.Check(expectedRemoteApp, jc.DeepEquals, &mockRemoteApplication{
		sourceModelUUID: coretesting.ModelTag.Id(), consumerproxy: true})
	expectedRel := s.st.relations["offeredapp:local remote-apptoken:remote"]
	expectedRel.Stub = testing.Stub{} // don't care about api calls
	c.Check(expectedRel, jc.DeepEquals, &mockRelation{id: 0, key: "offeredapp:local remote-apptoken:remote"})
	c.Check(s.st.remoteEntities, gc.HasLen, 2)
	c.Check(s.st.remoteEntities[names.NewApplicationTag("offered")], gc.Equals, "token-offered")
	c.Check(s.st.remoteEntities[names.NewRelationTag("offeredapp:local remote-apptoken:remote")], gc.Equals, "rel-token")
	c.Assert(s.st.offerConnections, gc.HasLen, 1)
	offerConnection := s.st.offerConnections[0]
	c.Assert(offerConnection, jc.DeepEquals, &mockOfferConnection{
		sourcemodelUUID: coretesting.ModelTag.Id(),
		relationId:      0,
		relationKey:     "offeredapp:local remote-apptoken:remote",
		username:        "mary",
		offerUUID:       "offer-uuid",
	})
}

func (s *crossmodelRelationsSuite) TestRegisterRemoteRelations(c *gc.C) {
	s.assertRegisterRemoteRelations(c)
}

func (s *crossmodelRelationsSuite) TestRegisterRemoteRelationsIdempotent(c *gc.C) {
	s.assertRegisterRemoteRelations(c)
	s.assertRegisterRemoteRelations(c)
}

func (s *crossmodelRelationsSuite) TestRelationUnitSettings(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.units["django/0"] = djangoRelationUnit
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "hosted-db2-uuid",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2"
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	// RelationUnitSettings isn't available on the V2 API.
	api := &crossmodelrelations.CrossModelRelationsAPIV1{s.api}
	result, err := api.RelationUnitSettings(params.RemoteRelationUnits{
		RelationUnits: []params.RemoteRelationUnit{{
			RelationToken: "token-db2",
			Unit:          "unit-django-0",
			Macaroons:     macaroon.Slice{mac.M()},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.SettingsResult{{Settings: params.Settings{"key": "value"}}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *crossmodelRelationsSuite) TestPublishIngressNetworkChanges(c *gc.C) {
	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	rel := newMockRelation(1)
	rel.key = "db2:db django:db"
	s.st.relations["db2:db django:db"] = rel
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "hosted-db2-uuid",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.PublishIngressNetworkChanges(params.IngressNetworksChanges{
		Changes: []params.IngressNetworksChangeEvent{
			{
				ApplicationToken: "token-db2",
				RelationToken:    "token-db2:db django:db",
				Networks:         []string{"1.2.3.4/32"},
				Macaroons:        macaroon.Slice{mac.M()},
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
	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.relations["db2:db django:db"] = newMockRelation(1)
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "hosted-db2-uuid",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	rule := state.NewFirewallRule("", []string{"10.1.1.1/8"})
	s.st.firewallRules[corefirewall.JujuApplicationOfferRule] = &rule
	results, err := s.api.PublishIngressNetworkChanges(params.IngressNetworksChanges{
		Changes: []params.IngressNetworksChangeEvent{
			{
				ApplicationToken: "token-db2",
				RelationToken:    "token-db2:db django:db",
				Networks:         []string{"1.2.3.4/32"},
				Macaroons:        macaroon.Slice{mac.M()},
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
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "hosted-db2-uuid",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
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
	results, err := s.api.WatchEgressAddressesForRelations(args)
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
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	rel := newMockRelation(1)
	s.st.relations["db2:db django:db"] = rel
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "hosted-db2-uuid",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
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
	results, err := s.api.WatchRelationsSuspendedStatus(args)
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
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "hosted-db2-uuid",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
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
	results, err := s.api.WatchRelationsSuspendedStatus(args)
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
	results, err = s.api.WatchRelationsSuspendedStatus(args)
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
	s.st.offers["mysql-uuid"] = &crossmodel.ApplicationOffer{
		OfferName: "hosted-mysql", OfferUUID: "mysql-uuid", ApplicationName: "mysql"}
	app := &mockApplication{}
	s.st.applications["mysql"] = app
	s.st.remoteEntities[names.NewApplicationOfferTag("hosted-mysql")] = "token-hosted-mysql"
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"mysql-uuid", "consume"})

	c.Assert(err, jc.ErrorIsNil)
	args := params.OfferArgs{
		Args: []params.OfferArg{
			{
				OfferUUID: "db2-uuid",
				Macaroons: macaroon.Slice{mac.M()},
			},
			{
				OfferUUID: "mysql-uuid",
				Macaroons: macaroon.Slice{mac.M()},
			},
			{
				OfferUUID: "postgresql-uuid",
				Macaroons: macaroon.Slice{mac.M()},
			},
		},
	}
	results, err := s.api.WatchOfferStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error.ErrorCode(), gc.Equals, params.CodeUnauthorized)
	c.Assert(results.Results[1].Error, gc.IsNil)
	c.Assert(results.Results[2].Error.ErrorCode(), gc.Equals, params.CodeUnauthorized)
	c.Assert(s.watchedOffers, jc.DeepEquals, []string{"mysql-uuid"})
	s.st.CheckCalls(c, []testing.StubCall{
		{"Application", []interface{}{"mysql"}},
	})
	app.CheckCalls(c, []testing.StubCall{
		{"Status", nil},
	})
}

func (s *crossmodelRelationsSuite) TestPublishChangesWithApplicationSettings(c *gc.C) {
	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	rel := newMockRelation(1)
	ru1 := newMockRelationUnit()
	ru2 := newMockRelationUnit()
	rel.units["db2/1"] = ru1
	rel.units["db2/2"] = ru2
	s.st.relations["db2:db django:db"] = rel
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "hosted-db2-uuid",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.PublishRelationChanges(params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{
			{
				Life:             life.Alive,
				ApplicationToken: "token-db2",
				RelationToken:    "token-db2:db django:db",
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
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2"}},
	}
	s.st.CheckCalls(c, expected)
	ru1.CheckCalls(c, []testing.StubCall{
		{"InScope", []interface{}{}},
		{"EnterScope", []interface{}{map[string]interface{}{"foo": "bar"}}},
	})
	ru2.CheckCalls(c, []testing.StubCall{
		{"LeaveScope", []interface{}{}},
	})
	rel.CheckCallNames(c, "Suspended", "ReplaceApplicationSettings", "RemoteUnit", "RemoteUnit")
	rel.CheckCall(c, 1, "ReplaceApplicationSettings", "db2", map[string]interface{}{
		"slaughterhouse": "the-tongue",
	})
}

func (s *crossmodelRelationsSuite) TestWatchRelationChanges(c *gc.C) {
	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	s.st.applications["django"] = &mockApplication{}
	s.st.remoteEntities[names.NewApplicationTag("offer-django")] = "token-offer-django"
	rel := newMockRelation(1)
	ru1 := newMockRelationUnit()
	ru2 := newMockRelationUnit()

	ru1.settings["che-fu"] = "fade away"

	rel.endpoints = append(rel.endpoints,
		state.Endpoint{ApplicationName: "db2"},
		state.Endpoint{ApplicationName: "django"},
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
		offerUUID:       "hosted-db2-uuid",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.offerNames["db2:db django:db"] = "offer-django"
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.WatchRelationChanges(params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{{
			Token:     "token-db2:db django:db",
			Macaroons: macaroon.Slice{mac.M()},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RemoteRelationWatchResults{
		Results: []params.RemoteRelationWatchResult{{
			RemoteRelationWatcherId: "1",
			Changes: params.RemoteRelationChangeEvent{
				RelationToken:    "token-db2:db django:db",
				ApplicationToken: "token-offer-django",
				Macaroons:        nil,
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
	defer statetesting.AssertStop(c, resource)

	outw, ok := resource.(*commoncrossmodel.WrappedUnitsWatcher)
	c.Assert(ok, gc.Equals, true)
	c.Assert(outw.RelationToken, gc.Equals, "token-db2:db django:db")
	c.Assert(outw.ApplicationToken, gc.Equals, "token-offer-django")

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

func (s *crossmodelRelationsSuite) TestWatchRelationUnitsOnV1(c *gc.C) {
	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	s.st.applications["django"] = &mockApplication{}
	s.st.remoteEntities[names.NewApplicationTag("django")] = "token-django"
	rel := newMockRelation(1)
	ru1 := newMockRelationUnit()
	ru2 := newMockRelationUnit()
	rel.endpoints = append(rel.endpoints,
		state.Endpoint{ApplicationName: "db2"},
		state.Endpoint{ApplicationName: "django"},
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
	}
	rel.watchers["django"] = w

	s.st.relations["db2:db django:db"] = rel
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerUUID:       "hosted-db2-uuid",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		}, bakery.Op{"db2:db django:db", "relate"})

	c.Assert(err, jc.ErrorIsNil)

	apiV1 := &crossmodelrelations.CrossModelRelationsAPIV1{s.api}

	result, err := apiV1.WatchRelationUnits(params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{{
			Token:     "token-db2:db django:db",
			Macaroons: macaroon.Slice{mac.M()},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RelationUnitsWatchResults{
		Results: []params.RelationUnitsWatchResult{{
			RelationUnitsWatcherId: "1",
			Changes: params.RelationUnitsChange{
				Changed: map[string]params.UnitSettings{
					"django/1": {Version: 100},
				},
			},
		}},
	})
}
