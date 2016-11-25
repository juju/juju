// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

type applicationDirectorySuite struct {
	ConnSuite
}

var _ = gc.Suite(&applicationDirectorySuite{})

func (s *applicationDirectorySuite) createDefaultOffer(c *gc.C) crossmodel.ApplicationOffer {
	eps := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
		{
			Interface: "mysql-root",
			Name:      "db-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	sd := state.NewApplicationDirectory(s.State)
	offer := crossmodel.ApplicationOffer{
		ApplicationURL:         "local:/u/me/application",
		ApplicationName:        "mysql",
		ApplicationDescription: "mysql is a db server",
		Endpoints:              eps,
		SourceModelUUID:        "source-model-uuid",
		SourceLabel:            "source",
	}
	err := sd.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)
	return offer
}

func (s *applicationDirectorySuite) TestEndpoints(c *gc.C) {
	offer := s.createDefaultOffer(c)
	_, err := state.ApplicationOfferEndpoint(offer, "foo")
	c.Assert(err, gc.ErrorMatches, `relation "foo" on application offer "source-model-uuid-mysql" not found`)

	serverEP, err := state.ApplicationOfferEndpoint(offer, "db")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})
}

func (s *applicationDirectorySuite) TestRemove(c *gc.C) {
	offer := s.createDefaultOffer(c)
	sd := state.NewApplicationDirectory(s.State)
	err := sd.Remove(offer.ApplicationURL)
	c.Assert(err, jc.ErrorIsNil)
	_, err = state.OfferAtURL(sd, offer.ApplicationURL)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *applicationDirectorySuite) TestAddApplicationOffer(c *gc.C) {
	eps := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
		{
			Interface: "mysql-root",
			Name:      "db-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	sd := state.NewApplicationDirectory(s.State)
	offer := crossmodel.ApplicationOffer{
		ApplicationURL:         "local:/u/me/application",
		ApplicationName:        "mysql",
		ApplicationDescription: "mysql is a db server",
		Endpoints:              eps,
		SourceModelUUID:        "source-model-uuid",
		SourceLabel:            "source",
	}
	err := sd.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)
	expectedDoc := state.MakeApplicationOfferDoc(sd, "local:/u/me/application", offer)
	doc, err := state.OfferAtURL(sd, "local:/u/me/application")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*doc, jc.DeepEquals, expectedDoc)
}

func (s *applicationDirectorySuite) TestListOffersNone(c *gc.C) {
	sd := state.NewApplicationDirectory(s.State)
	offers, err := sd.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 0)
}

func (s *applicationDirectorySuite) createOffer(c *gc.C, name, description, uuid, label string) crossmodel.ApplicationOffer {
	eps := []charm.Relation{
		{
			Interface: name + "-interface",
			Name:      name,
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
		{
			Interface: name + "-interface2",
			Name:      name + "2",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		},
	}
	sd := state.NewApplicationDirectory(s.State)
	offer := crossmodel.ApplicationOffer{
		ApplicationURL:         "local:/u/me/" + name,
		ApplicationName:        name,
		ApplicationDescription: description,
		Endpoints:              eps,
		SourceModelUUID:        uuid,
		SourceLabel:            label,
	}
	err := sd.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)
	return offer
}

func (s *applicationDirectorySuite) TestListOffersAll(c *gc.C) {
	sd := state.NewApplicationDirectory(s.State)
	offer := s.createDefaultOffer(c)
	offers, err := sd.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *applicationDirectorySuite) TestListOffersOneFilter(c *gc.C) {
	sd := state.NewApplicationDirectory(s.State)
	offer := s.createOffer(c, "offer1", "description for offer1", "uuid-1", "label")
	s.createOffer(c, "offer2", "description for offer2", "uuid-2", "label")
	s.createOffer(c, "offer3", "description for offer3", "uuid-3", "label")
	offers, err := sd.ListOffers(crossmodel.ApplicationOfferFilter{
		ApplicationOffer: crossmodel.ApplicationOffer{
			ApplicationURL:  "local:/u/me/offer1",
			ApplicationName: "offer1",
			SourceModelUUID: "uuid-1",
			SourceLabel:     "label",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *applicationDirectorySuite) TestListOffersManyFilters(c *gc.C) {
	sd := state.NewApplicationDirectory(s.State)
	offer := s.createOffer(c, "offer1", "description for offer1", "uuid-1", "label")
	offer2 := s.createOffer(c, "offer2", "description for offer2", "uuid-2", "label")
	s.createOffer(c, "offer3", "description for offer3", "uuid-3", "label")
	offers, err := sd.ListOffers(
		crossmodel.ApplicationOfferFilter{
			ApplicationOffer: crossmodel.ApplicationOffer{
				ApplicationURL:  "local:/u/me/offer1",
				ApplicationName: "offer1",
				SourceModelUUID: "uuid-1",
				SourceLabel:     "label",
			}},
		crossmodel.ApplicationOfferFilter{
			ApplicationOffer: crossmodel.ApplicationOffer{
				ApplicationURL:         "local:/u/me/offer2",
				ApplicationDescription: "offer2",
				SourceLabel:            "label",
			}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 2)
	c.Assert(offers, jc.DeepEquals, []crossmodel.ApplicationOffer{offer, offer2})
}

func (s *applicationDirectorySuite) TestListOffersFilterDescriptionRegexp(c *gc.C) {
	sd := state.NewApplicationDirectory(s.State)
	s.createOffer(c, "offer1", "description for offer1", "uuid-1", "label")
	offer := s.createOffer(c, "offer2", "description for offer2", "uuid-2", "label")
	s.createOffer(c, "offer3", "description for offer3", "uuid-3", "label")
	offers, err := sd.ListOffers(crossmodel.ApplicationOfferFilter{
		ApplicationOffer: crossmodel.ApplicationOffer{
			ApplicationDescription: "for offer2",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *applicationDirectorySuite) TestListOffersFilterApplicationURLRegexp(c *gc.C) {
	sd := state.NewApplicationDirectory(s.State)
	s.createOffer(c, "offer1", "description for offer1", "uuid-1", "label")
	offer := s.createOffer(c, "offer2", "description for offer2", "uuid-2", "label")
	s.createOffer(c, "offer3", "description for offer3", "uuid-3", "label")
	offers, err := sd.ListOffers(crossmodel.ApplicationOfferFilter{
		ApplicationOffer: crossmodel.ApplicationOffer{
			ApplicationURL: "me/offer2",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *applicationDirectorySuite) TestAddApplicationOfferUUIDRequired(c *gc.C) {
	sd := state.NewApplicationDirectory(s.State)
	err := sd.AddOffer(crossmodel.ApplicationOffer{
		ApplicationURL:  "local:/u/me/application",
		ApplicationName: "mysql",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application offer "mysql" at "local:/u/me/application": missing source model UUID`)
}

func (s *applicationDirectorySuite) TestAddApplicationOfferDuplicate(c *gc.C) {
	sd := state.NewApplicationDirectory(s.State)
	err := sd.AddOffer(crossmodel.ApplicationOffer{
		ApplicationURL:  "local:/u/me/application",
		ApplicationName: "mysql",
		SourceModelUUID: "uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = sd.AddOffer(crossmodel.ApplicationOffer{
		ApplicationURL:  "local:/u/me/application",
		ApplicationName: "another",
		SourceModelUUID: "uuid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application offer "another" at "local:/u/me/application": application offer already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationOfferDuplicateAddedAfterInitial(c *gc.C) {
	// Check that a record with a URL conflict cannot be added if
	// there is no conflict initially but a record is added
	// before the transaction is run.
	sd := state.NewApplicationDirectory(s.State)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := sd.AddOffer(crossmodel.ApplicationOffer{
			ApplicationURL:  "local:/u/me/application",
			ApplicationName: "mysql",
			SourceModelUUID: "uuid",
		})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err := sd.AddOffer(crossmodel.ApplicationOffer{
		ApplicationURL:  "local:/u/me/application",
		ApplicationName: "mysql",
		SourceModelUUID: "uuid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application offer "mysql" at "local:/u/me/application": application offer already exists`)
}

func (s *applicationDirectorySuite) TestUpdateApplicationOfferUUIDRequired(c *gc.C) {
	sd := state.NewApplicationDirectory(s.State)
	err := sd.UpdateOffer(crossmodel.ApplicationOffer{
		ApplicationURL:  "local:/u/me/application",
		ApplicationName: "mysql",
	})
	c.Assert(err, gc.ErrorMatches, `cannot update application offer "mysql": missing source model UUID`)
}

func (s *applicationDirectorySuite) TestUpdateApplicationOfferNotFound(c *gc.C) {
	sd := state.NewApplicationDirectory(s.State)
	err := sd.UpdateOffer(crossmodel.ApplicationOffer{
		ApplicationURL:  "local:/u/me/application",
		ApplicationName: "mysql",
		SourceModelUUID: "uuid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot update application offer "mysql": application offer "local:/u/me/application" not found`)
}

func (s *remoteApplicationSuite) TestUpdateApplicationOfferRemovedAfterInitial(c *gc.C) {
	// Check that a record with a URL conflict cannot be added if
	// there is no conflict initially but a record is added
	// before the transaction is run.
	sd := state.NewApplicationDirectory(s.State)
	err := sd.AddOffer(crossmodel.ApplicationOffer{
		ApplicationURL:  "local:/u/me/application",
		ApplicationName: "mysql",
		SourceModelUUID: "uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := sd.Remove("local:/u/me/application")
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err = sd.UpdateOffer(crossmodel.ApplicationOffer{
		ApplicationURL:  "local:/u/me/application",
		ApplicationName: "mysql",
		SourceModelUUID: "uuid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot update application offer "mysql": application offer "local:/u/me/application" not found`)
}
