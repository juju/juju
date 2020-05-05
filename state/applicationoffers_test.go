// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type applicationOffersSuite struct {
	ConnSuite
	mysql *state.Application
}

var _ = gc.Suite(&applicationOffersSuite{})

func (s *applicationOffersSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	ch := s.AddTestingCharm(c, "mysql")
	s.mysql = s.AddTestingApplication(c, "mysql", ch)
}

func (s *applicationOffersSuite) createDefaultOffer(c *gc.C) crossmodel.ApplicationOffer {
	eps := map[string]string{"db": "server", "db-admin": "server-admin"}
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	offerArgs := crossmodel.AddApplicationOfferArgs{
		OfferName:              "hosted-mysql",
		ApplicationName:        "mysql",
		ApplicationDescription: "mysql is a db server",
		Endpoints:              eps,
		Owner:                  owner.Name(),
	}
	offer, err := sd.AddOffer(offerArgs)
	c.Assert(err, jc.ErrorIsNil)
	return *offer
}

func (s *applicationOffersSuite) TestDetectingOfferConnections(c *gc.C) {
	connected, err := state.ApplicationHasConnectedOffers(s.State, "mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(connected, jc.IsFalse)

	offer := s.createDefaultOffer(c)
	connected, err = state.ApplicationHasConnectedOffers(s.State, "mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(connected, jc.IsFalse)

	s.addOfferConnection(c, offer.OfferUUID)
	connected, err = state.ApplicationHasConnectedOffers(s.State, "mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(connected, jc.IsTrue)
}

func (s *applicationOffersSuite) TestEndpoints(c *gc.C) {
	offer := s.createDefaultOffer(c)
	_, err := state.ApplicationOfferEndpoint(offer, "foo")
	c.Assert(err, gc.ErrorMatches, `relation "foo" on application offer "mysql" not found`)

	serverEP, err := state.ApplicationOfferEndpoint(offer, "server")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})
}

func (s *applicationOffersSuite) TestRemove(c *gc.C) {
	offer := s.createDefaultOffer(c)
	sd := state.NewApplicationOffers(s.State)
	err := sd.Remove(offer.OfferName, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = sd.ApplicationOffer(offer.OfferName)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *applicationOffersSuite) TestAddApplicationOffer(c *gc.C) {
	eps := map[string]string{"db": "server", "db-admin": "server-admin"}
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	args := crossmodel.AddApplicationOfferArgs{
		OfferName:              "hosted-mysql",
		ApplicationName:        "mysql",
		ApplicationDescription: "mysql is a db server",
		Endpoints:              eps,
		Owner:                  owner.Name(),
		HasRead:                []string{"everyone@external"},
	}
	offer, err := sd.AddOffer(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedOffer, err := sd.ApplicationOffer(offer.OfferName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*offer, jc.DeepEquals, *expectedOffer)

	access, err := s.State.GetOfferAccess(offer.OfferUUID, owner.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AdminAccess)

	access, err = s.State.GetOfferAccess(offer.OfferUUID, names.NewUserTag("everyone@external"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}

func (s *applicationOffersSuite) TestAddApplicationOfferBadEndpoints(c *gc.C) {
	eps := map[string]string{"db": "server", "db-admin": "admin"}
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	args := crossmodel.AddApplicationOfferArgs{
		OfferName:              "hosted-mysql",
		ApplicationName:        "mysql",
		ApplicationDescription: "mysql is a db server",
		Endpoints:              eps,
		Owner:                  owner.Name(),
	}
	_, err := sd.AddOffer(args)
	c.Assert(err, gc.ErrorMatches, `.*application "mysql" has no "admin" relation`)

	// Fix the endpoints and try again.
	// There was a bug where this failed so we test it.
	eps = map[string]string{"db": "server", "db-admin": "server-admin"}
	args.Endpoints = eps
	_, err = sd.AddOffer(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationOffersSuite) TestListOffersNone(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	offers, err := sd.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 0)
}

func (s *applicationOffersSuite) createOffer(c *gc.C, name, description string) (crossmodel.ApplicationOffer, string) {
	eps := map[string]string{
		"db": "server",
	}
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	offerArgs := crossmodel.AddApplicationOfferArgs{
		OfferName:              name,
		ApplicationName:        "mysql",
		ApplicationDescription: description,
		Endpoints:              eps,
		Owner:                  owner.Name(),
	}
	offer, err := sd.AddOffer(offerArgs)
	c.Assert(err, jc.ErrorIsNil)
	return *offer, owner.Name()
}

func (s *applicationOffersSuite) TestApplicationOffer(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	expectedOffer := s.createDefaultOffer(c)
	offer, err := sd.ApplicationOffer("hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*offer, jc.DeepEquals, expectedOffer)
}

func (s *applicationOffersSuite) TestApplicationOfferForUUID(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	expectedOffer := s.createDefaultOffer(c)
	offer, err := sd.ApplicationOfferForUUID(expectedOffer.OfferUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*offer, jc.DeepEquals, expectedOffer)
}

func (s *applicationOffersSuite) TestAllApplicationOffers(c *gc.C) {
	eps := map[string]string{"db": "server", "db-admin": "server-admin"}
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	anOffer := s.createDefaultOffer(c)
	args := crossmodel.AddApplicationOfferArgs{
		OfferName:              "another-mysql",
		ApplicationName:        "mysql",
		ApplicationDescription: "mysql is a db server",
		Endpoints:              eps,
		Owner:                  owner.Name(),
		HasRead:                []string{"everyone@external"},
	}
	anotherOffer, err := sd.AddOffer(args)
	c.Assert(err, jc.ErrorIsNil)

	offers, err := sd.AllApplicationOffers()
	c.Assert(err, jc.ErrorIsNil)
	// Ensure ordering doesn't matter.
	offersMap := make(map[string]*crossmodel.ApplicationOffer)
	for _, offer := range offers {
		offersMap[offer.OfferName] = offer
	}
	c.Assert(offersMap, jc.DeepEquals, map[string]*crossmodel.ApplicationOffer{
		anOffer.OfferName:      &anOffer,
		anotherOffer.OfferName: anotherOffer,
	})
}

func (s *applicationOffersSuite) TestListOffersAll(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	offer := s.createDefaultOffer(c)
	offers, err := sd.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *applicationOffersSuite) TestListOffersOneFilter(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	offer, _ := s.createOffer(c, "offer1", "description for offer1")
	s.createOffer(c, "offer2", "description for offer2")
	s.createOffer(c, "offer3", "description for offer3")
	offers, err := sd.ListOffers(crossmodel.ApplicationOfferFilter{
		OfferName:       "offer1",
		ApplicationName: "mysql",
		Endpoints: []crossmodel.EndpointFilterTerm{{
			Interface: "mysql",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *applicationOffersSuite) TestListOffersExact(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	offer, _ := s.createOffer(c, "offer1", "description for offer1")
	s.createOffer(c, "offer2", "description for offer2")
	s.createOffer(c, "offer3", "description for offer3")
	offers, err := sd.ListOffers(crossmodel.ApplicationOfferFilter{
		OfferName: "^offer1$",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
	offers, err = sd.ListOffers(crossmodel.ApplicationOfferFilter{
		OfferName: "^offer$",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 0)
}

func (s *applicationOffersSuite) TestListOffersFilterExcludes(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	s.createOffer(c, "offer1", "description for offer1")
	offers, err := sd.ListOffers(crossmodel.ApplicationOfferFilter{
		Endpoints: []crossmodel.EndpointFilterTerm{{
			Interface: "db2",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 0)
}

func (s *applicationOffersSuite) TestListOffersManyFilters(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	offer, _ := s.createOffer(c, "offer1", "description for offer1")
	offer2, _ := s.createOffer(c, "offer2", "description for offer2")
	s.createOffer(c, "offer3", "description for offer3")
	offers, err := sd.ListOffers(
		crossmodel.ApplicationOfferFilter{
			OfferName:       "offer1",
			ApplicationName: "mysql",
		},
		crossmodel.ApplicationOfferFilter{
			OfferName:              "offer2",
			ApplicationDescription: "offer2",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 2)
	c.Assert(offers, jc.DeepEquals, []crossmodel.ApplicationOffer{offer, offer2})
}

func (s *applicationOffersSuite) TestListOffersFilterDescriptionRegexp(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	s.createOffer(c, "offer1", "description for offer1")
	offer, _ := s.createOffer(c, "offer2", "description for offer2")
	s.createOffer(c, "offer3", "description for offer3")
	offers, err := sd.ListOffers(crossmodel.ApplicationOfferFilter{
		ApplicationDescription: "for offer2",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *applicationOffersSuite) TestListOffersFilterOfferNameRegexp(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	offer, _ := s.createOffer(c, "hosted-offer1", "description for offer1")
	s.createOffer(c, "offer2", "description for offer2")
	s.createOffer(c, "offer3", "description for offer3")
	offers, err := sd.ListOffers(crossmodel.ApplicationOfferFilter{
		OfferName: "offer1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *applicationOffersSuite) TestListOffersAllowedConsumersOwner(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	offer, owner := s.createOffer(c, "offer1", "description for offer1")
	s.createOffer(c, "offer2", "description for offer2")
	s.createOffer(c, "offer3", "description for offer3")
	offers, err := sd.ListOffers(crossmodel.ApplicationOfferFilter{
		AllowedConsumers: []string{owner, "mary"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *applicationOffersSuite) TestListOffersAllowedConsumers(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	offer, _ := s.createOffer(c, "offer1", "description for offer1")
	offer2, _ := s.createOffer(c, "offer2", "description for offer2")
	s.createOffer(c, "offer3", "description for offer3")
	s.Factory.MakeUser(c, &factory.UserParams{Name: "mary"})

	mary := names.NewUserTag("mary")
	err := s.State.CreateOfferAccess(
		names.NewApplicationOfferTag(offer.OfferName), mary, permission.ConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.CreateOfferAccess(
		names.NewApplicationOfferTag(offer2.OfferName), mary, permission.ReadAccess)
	c.Assert(err, jc.ErrorIsNil)
	offers, err := sd.ListOffers(crossmodel.ApplicationOfferFilter{
		AllowedConsumers: []string{"mary"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *applicationOffersSuite) TestListOffersConnectedUsers(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	offer, _ := s.createOffer(c, "offer1", "description for offer1")
	s.createOffer(c, "offer2", "description for offer2")
	s.createOffer(c, "offer3", "description for offer3")
	s.Factory.MakeUser(c, &factory.UserParams{Name: "mary"})

	_, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		Username:        "mary",
		OfferUUID:       offer.OfferUUID,
	})
	c.Assert(err, jc.ErrorIsNil)
	offers, err := sd.ListOffers(crossmodel.ApplicationOfferFilter{
		ConnectedUsers: []string{"mary"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *applicationOffersSuite) TestAddApplicationOfferDuplicate(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	_, err := sd.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           owner.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = sd.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           owner.Name(),
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application offer "hosted-mysql": application offer already exists`)
}

func (s *applicationOffersSuite) TestAddApplicationOfferDuplicateAddedAfterInitial(c *gc.C) {
	// Check that a record with a URL conflict cannot be added if
	// there is no conflict initially but a record is added
	// before the transaction is run.
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := sd.AddOffer(crossmodel.AddApplicationOfferArgs{
			OfferName:       "hosted-mysql",
			ApplicationName: "mysql",
			Owner:           owner.Name(),
		})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := sd.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           owner.Name(),
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application offer "hosted-mysql": application offer already exists`)
}

func (s *applicationOffersSuite) TestUpdateApplicationOffer(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	original, err := sd.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           owner.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	offer, err := sd.UpdateOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:              "hosted-mysql",
		ApplicationName:        "mysql",
		ApplicationDescription: "a better database",
		Owner:                  owner.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offer, jc.DeepEquals, &crossmodel.ApplicationOffer{
		OfferName:              "hosted-mysql",
		OfferUUID:              original.OfferUUID,
		ApplicationName:        "mysql",
		ApplicationDescription: "a better database",
		Endpoints:              map[string]charm.Relation{},
	})
	assertOffersRef(c, s.State, "mysql", 1)
}

func (s *applicationOffersSuite) TestUpdateApplicationOfferDifferentApp(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	original, err := sd.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           owner.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "foo"})
	offer, err := sd.UpdateOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "foo",
		Owner:           owner.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offer, jc.DeepEquals, &crossmodel.ApplicationOffer{
		OfferName:       "hosted-mysql",
		OfferUUID:       original.OfferUUID,
		ApplicationName: "foo",
		Endpoints:       map[string]charm.Relation{},
	})
	assertNoOffersRef(c, s.State, "mysql")
	assertOffersRef(c, s.State, "foo", 1)
}

func (s *applicationOffersSuite) TestUpdateApplicationOfferNotFound(c *gc.C) {
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	_, err := sd.UpdateOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "foo",
		Owner:           owner.Name(),
	})
	c.Assert(err, gc.ErrorMatches, `cannot update application offer "foo": application offer "hosted-mysql" not found`)
}

func (s *applicationOffersSuite) TestUpdateApplicationOfferRemovedAfterInitial(c *gc.C) {
	// Check that a record with a URL conflict cannot be added if
	// there is no conflict initially but a record is added
	// before the transaction is run.
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	_, err := sd.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           owner.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := sd.Remove("hosted-mysql", false)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err = sd.UpdateOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           owner.Name(),
	})
	c.Assert(err, gc.ErrorMatches, `cannot update application offer "mysql": application offer "hosted-mysql" not found`)
}

func (s *applicationOffersSuite) addOfferConnection(c *gc.C, offerUUID string) *state.RemoteApplication {
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "wordpress",
		SourceModel: testing.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		OfferUUID:       offerUUID,
		RelationId:      rel.Id(),
		RelationKey:     rel.Tag().Id(),
		Username:        "admin",
		SourceModelUUID: testing.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	return app
}

func (s *applicationOffersSuite) TestRemoveOffersSucceedsWithZeroConnections(c *gc.C) {
	s.createDefaultOffer(c)
	ao := state.NewApplicationOffers(s.State)
	err := ao.Remove("hosted-mysql", false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ao.ApplicationOffer("hosted-mysql")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertNoOffersRef(c, s.State, "mysql")
}

func (s *applicationOffersSuite) TestRemoveApplicationSucceedsWithZeroConnections(c *gc.C) {
	s.createDefaultOffer(c)

	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	assertNoOffersRef(c, s.State, "mysql")
}

func (s *applicationOffersSuite) TestRemoveApplicationSucceedsWithZeroConnectionsRace(c *gc.C) {
	addOffer := func() {
		s.createDefaultOffer(c)
	}
	defer state.SetBeforeHooks(c, s.State, addOffer).Check()
	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	assertNoOffersRef(c, s.State, "mysql")
}

func (s *applicationOffersSuite) TestRemoveApplicationFailsWithOfferWithConnections(c *gc.C) {
	offer := s.createDefaultOffer(c)
	s.addOfferConnection(c, offer.OfferUUID)

	err := s.mysql.Destroy()
	c.Assert(err, gc.ErrorMatches, `cannot destroy application "mysql": application is used by 1 offer`)
	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 1)
}

func (s *applicationOffersSuite) TestRemoveApplicationFailsWithOfferWithConnectionsRace(c *gc.C) {
	addConnectedOffer := func() {
		offer := s.createDefaultOffer(c)
		s.addOfferConnection(c, offer.OfferUUID)
	}
	defer state.SetBeforeHooks(c, s.State, addConnectedOffer).Check()
	err := s.mysql.Destroy()
	c.Assert(err, gc.ErrorMatches, `cannot destroy application "mysql": application is used by 1 offer`)
	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 1)
}

func (s *applicationOffersSuite) TestRemoveOffersFailsWithConnections(c *gc.C) {
	offer := s.createDefaultOffer(c)
	s.addOfferConnection(c, offer.OfferUUID)
	ao := state.NewApplicationOffers(s.State)
	err := ao.Remove("hosted-mysql", false)
	c.Assert(err, gc.ErrorMatches, `cannot delete application offer "hosted-mysql": offer has 1 relation`)
}

func (s *applicationOffersSuite) TestRemoveOffersFailsWithConnectionsRace(c *gc.C) {
	offer := s.createDefaultOffer(c)
	ao := state.NewApplicationOffers(s.State)
	addOfferConnection := func() {
		c.Logf("adding connection to %s", offer.OfferUUID)
		s.addOfferConnection(c, offer.OfferUUID)
	}
	defer state.SetBeforeHooks(c, s.State, addOfferConnection).Check()

	err := ao.Remove("hosted-mysql", false)
	c.Assert(err, gc.ErrorMatches, `cannot delete application offer "hosted-mysql": offer has 1 relation`)
}

func (s *applicationOffersSuite) TestRemoveOffersSucceedsWhenLocalRelationAdded(c *gc.C) {
	offer := s.createDefaultOffer(c)
	s.AddTestingApplication(c, "local-wordpress", s.AddTestingCharm(c, "wordpress"))
	_, err := s.State.Application(offer.ApplicationName)
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("local-wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ao := state.NewApplicationOffers(s.State)

	err = ao.Remove(offer.OfferName, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ao.ApplicationOffer("hosted-mysql")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *applicationOffersSuite) assertInScope(c *gc.C, relUnit *state.RelationUnit, inScope bool) {
	ok, err := relUnit.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, gc.Equals, inScope)
}

func (s *applicationOffersSuite) TestRemoveOffersWithConnectionsForce(c *gc.C) {
	offer := s.createDefaultOffer(c)
	rwordpress, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "remote-wordpress",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	wordpress, err := s.State.RemoteApplication("remote-wordpress")
	c.Assert(err, jc.ErrorIsNil)
	wordpressEP, err := rwordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	mysql, err := s.State.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	mysqlUnit, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	rel, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	mysqlru, err := rel.Unit(mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlru, true)

	wpru, err := rel.RemoteUnit("remote-wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	err = wpru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpru, true)

	s.addOfferConnection(c, offer.OfferUUID)
	ao := state.NewApplicationOffers(s.State)
	err = ao.Remove("hosted-mysql", true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ao.ApplicationOffer("hosted-mysql")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	conn, err := s.State.OfferConnections(offer.OfferUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conn, gc.HasLen, 0)
	s.assertInScope(c, wpru, false)
	s.assertInScope(c, mysqlru, true)
	err = wordpress.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, wordpress, state.Dying)
}

func (s *applicationOffersSuite) TestRemovingApplicationFailsRace(c *gc.C) {
	s.createDefaultOffer(c)
	wp := s.AddTestingApplication(c, "local-wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints(wp.Name(), s.mysql.Name())
	c.Assert(err, jc.ErrorIsNil)

	addRelation := func() {
		_, err := s.State.AddRelation(eps...)
		c.Assert(err, jc.ErrorIsNil)
	}

	rmRelations := func() {
		rels, err := s.State.AllRelations()
		c.Assert(err, jc.ErrorIsNil)

		for _, rel := range rels {
			err = rel.Destroy()
			c.Assert(err, jc.ErrorIsNil)
			err = s.mysql.Refresh()
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	bumpTxnRevno := txn.TestHook{Before: addRelation, After: rmRelations}
	defer state.SetTestHooks(c, s.State, bumpTxnRevno, bumpTxnRevno, bumpTxnRevno).Check()

	err = s.mysql.Destroy()
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(err, gc.ErrorMatches, "cannot destroy application.*")
	s.mysql.Refresh()
	assertOffersRef(c, s.State, "mysql", 1)
}

func (s *applicationOffersSuite) TestRemoveOffersWithConnectionsRace(c *gc.C) {
	// Create a local wordpress application to relate to the local mysql,
	// to show that we count remote relations correctly.
	s.AddTestingApplication(c, "local-wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("local-wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	localRel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	ao := state.NewApplicationOffers(s.State)
	offer := s.createDefaultOffer(c)
	addOfferConnection := func() {
		// Remove the local relation and add a remote relation,
		// so that the relation count remains stable. We should
		// be checking the *remote* relation count.
		c.Assert(localRel.Destroy(), jc.ErrorIsNil)
		s.addOfferConnection(c, offer.OfferUUID)
	}
	defer state.SetBeforeHooks(c, s.State, addOfferConnection).Check()

	err = ao.Remove(offer.OfferName, false)
	c.Assert(err, gc.ErrorMatches, `cannot delete application offer "hosted-mysql": offer has 1 relation`)
}

func (s *applicationOffersSuite) TestWatchOfferStatus(c *gc.C) {
	ao := state.NewApplicationOffers(s.State)
	offer, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w, err := s.State.WatchOfferStatus(offer.OfferUUID)
	c.Assert(err, jc.ErrorIsNil)

	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	// Initial event.
	wc.AssertOneChange()
	wc.AssertNoChange()

	app, err := s.State.Application(offer.ApplicationName)
	c.Assert(err, jc.ErrorIsNil)
	err = app.SetStatus(status.StatusInfo{
		Status:  status.Waiting,
		Message: "waiting for replication",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
	wc.AssertNoChange()

	u := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	err = u.SetStatus(status.StatusInfo{
		Status: status.Blocked,
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
	wc.AssertNoChange()
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
	wc.AssertNoChange()

	err = ao.Remove(offer.OfferName, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ao.ApplicationOffer("hosted-mysql")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
	wc.AssertNoChange()
}
