// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type applicationOffersSuite struct {
	ConnSuite
}

var _ = gc.Suite(&applicationOffersSuite{})

func (s *applicationOffersSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	ch := s.AddTestingCharm(c, "mysql")
	s.AddTestingApplication(c, "mysql", ch)
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
	err := sd.Remove(offer.OfferName)
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
		Owner: owner.Name(),
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
		err := sd.Remove("hosted-mysql")
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err = sd.UpdateOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           owner.Name(),
	})
	c.Assert(err, gc.ErrorMatches, `cannot update application offer "mysql": application offer "hosted-mysql" not found`)
}

func (s *applicationOffersSuite) addOfferConnection(c *gc.C, offerUUID string) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
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
}

func (s *applicationOffersSuite) TestRemoveOffersWithConnections(c *gc.C) {
	offer := s.createDefaultOffer(c)
	s.addOfferConnection(c, offer.OfferUUID)
	ao := state.NewApplicationOffers(s.State)
	err := ao.Remove("hosted-mysql")
	c.Assert(err, gc.ErrorMatches, `cannot delete application offer "hosted-mysql": offer has 1 relation`)
}

func (s *applicationOffersSuite) TestRemoveOffersWithConnectionsRace(c *gc.C) {
	ao := state.NewApplicationOffers(s.State)
	offer := s.createDefaultOffer(c)
	addOfferConnection := func() {
		s.addOfferConnection(c, offer.OfferUUID)
	}
	defer state.SetBeforeHooks(c, s.State, addOfferConnection).Check()

	err := ao.Remove(offer.OfferName)
	c.Assert(err, gc.ErrorMatches, `cannot delete application offer "hosted-mysql": offer has 1 relation`)
}
