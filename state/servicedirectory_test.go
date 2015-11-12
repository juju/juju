// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
)

type serviceDirectorySuite struct {
	ConnSuite
}

var _ = gc.Suite(&serviceDirectorySuite{})

func (s *serviceDirectorySuite) createDefaultOffer(c *gc.C) crossmodel.ServiceOffer {
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
	sd := state.NewServiceDirectory(s.State)
	offer := crossmodel.ServiceOffer{
		ServiceURL:         "local:/u/me/service",
		ServiceName:        "mysql",
		ServiceDescription: "mysql is a db server",
		Endpoints:          eps,
		SourceEnvUUID:      "source-uuid",
		SourceLabel:        "source",
	}
	err := sd.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)
	return offer
}

func (s *serviceDirectorySuite) TestEndpoints(c *gc.C) {
	offer := s.createDefaultOffer(c)
	_, err := state.ServiceOfferEndpoint(offer, "foo")
	c.Assert(err, gc.ErrorMatches, `relation "foo" on service offer "source-uuid-mysql" not found`)

	serverEP, err := state.ServiceOfferEndpoint(offer, "db")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverEP, gc.DeepEquals, state.Endpoint{
		ServiceName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})
}

func (s *serviceDirectorySuite) TestRemove(c *gc.C) {
	offer := s.createDefaultOffer(c)
	sd := state.NewServiceDirectory(s.State)
	err := sd.Remove(offer.ServiceURL)
	c.Assert(err, jc.ErrorIsNil)
	_, err = state.OfferAtURL(sd, offer.ServiceURL)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *serviceDirectorySuite) TestAddServiceOffer(c *gc.C) {
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
	sd := state.NewServiceDirectory(s.State)
	offer := crossmodel.ServiceOffer{
		ServiceURL:         "local:/u/me/service",
		ServiceName:        "mysql",
		ServiceDescription: "mysql is a db server",
		Endpoints:          eps,
		SourceEnvUUID:      "source-uuid",
		SourceLabel:        "source",
	}
	err := sd.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)
	expectedDoc := state.MakeServiceOfferDoc(sd, "local:/u/me/service", offer)
	doc, err := state.OfferAtURL(sd, "local:/u/me/service")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*doc, jc.DeepEquals, expectedDoc)
}

func (s *serviceDirectorySuite) TestListOffersNone(c *gc.C) {
	sd := state.NewServiceDirectory(s.State)
	offers, err := sd.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 0)
}

func (s *serviceDirectorySuite) createOffer(c *gc.C, name, description, uuid, label string) crossmodel.ServiceOffer {
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
	sd := state.NewServiceDirectory(s.State)
	offer := crossmodel.ServiceOffer{
		ServiceURL:         "local:/u/me/" + name,
		ServiceName:        name,
		ServiceDescription: description,
		Endpoints:          eps,
		SourceEnvUUID:      uuid,
		SourceLabel:        label,
	}
	err := sd.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)
	return offer
}

func (s *serviceDirectorySuite) TestListOffersAll(c *gc.C) {
	sd := state.NewServiceDirectory(s.State)
	offer := s.createDefaultOffer(c)
	offers, err := sd.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *serviceDirectorySuite) TestListOffersOneFilter(c *gc.C) {
	sd := state.NewServiceDirectory(s.State)
	offer := s.createOffer(c, "offer1", "description for offer1", "uuid-1", "label")
	s.createOffer(c, "offer2", "description for offer2", "uuid-2", "label")
	s.createOffer(c, "offer3", "description for offer3", "uuid-3", "label")
	offers, err := sd.ListOffers(crossmodel.ServiceOfferFilter{
		ServiceOffer: crossmodel.ServiceOffer{
			ServiceURL:    "local:/u/me/offer1",
			ServiceName:   "offer1",
			SourceEnvUUID: "uuid-1",
			SourceLabel:   "label",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *serviceDirectorySuite) TestListOffersManyFilters(c *gc.C) {
	sd := state.NewServiceDirectory(s.State)
	offer := s.createOffer(c, "offer1", "description for offer1", "uuid-1", "label")
	offer2 := s.createOffer(c, "offer2", "description for offer2", "uuid-2", "label")
	s.createOffer(c, "offer3", "description for offer3", "uuid-3", "label")
	offers, err := sd.ListOffers(
		crossmodel.ServiceOfferFilter{
			ServiceOffer: crossmodel.ServiceOffer{
				ServiceURL:    "local:/u/me/offer1",
				ServiceName:   "offer1",
				SourceEnvUUID: "uuid-1",
				SourceLabel:   "label",
			}},
		crossmodel.ServiceOfferFilter{
			ServiceOffer: crossmodel.ServiceOffer{
				ServiceURL:         "local:/u/me/offer2",
				ServiceDescription: "offer2",
				SourceLabel:        "label",
			}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 2)
	c.Assert(offers, jc.DeepEquals, []crossmodel.ServiceOffer{offer, offer2})
}

func (s *serviceDirectorySuite) TestListOffersFilterDescriptionRegexp(c *gc.C) {
	sd := state.NewServiceDirectory(s.State)
	s.createOffer(c, "offer1", "description for offer1", "uuid-1", "label")
	offer := s.createOffer(c, "offer2", "description for offer2", "uuid-2", "label")
	s.createOffer(c, "offer3", "description for offer3", "uuid-3", "label")
	offers, err := sd.ListOffers(crossmodel.ServiceOfferFilter{
		ServiceOffer: crossmodel.ServiceOffer{
			ServiceDescription: "for offer2",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *serviceDirectorySuite) TestListOffersFilterServiceURLRegexp(c *gc.C) {
	sd := state.NewServiceDirectory(s.State)
	s.createOffer(c, "offer1", "description for offer1", "uuid-1", "label")
	offer := s.createOffer(c, "offer2", "description for offer2", "uuid-2", "label")
	s.createOffer(c, "offer3", "description for offer3", "uuid-3", "label")
	offers, err := sd.ListOffers(crossmodel.ServiceOfferFilter{
		ServiceOffer: crossmodel.ServiceOffer{
			ServiceURL: "me/offer2",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *serviceDirectorySuite) TestAddServiceOfferUUIDRequired(c *gc.C) {
	sd := state.NewServiceDirectory(s.State)
	err := sd.AddOffer(crossmodel.ServiceOffer{
		ServiceURL:  "local:/u/me/service",
		ServiceName: "mysql",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add service offer "mysql" at "local:/u/me/service": missing source environment UUID`)
}

func (s *serviceDirectorySuite) TestAddServiceOfferDuplicate(c *gc.C) {
	sd := state.NewServiceDirectory(s.State)
	err := sd.AddOffer(crossmodel.ServiceOffer{
		ServiceURL:    "local:/u/me/service",
		ServiceName:   "mysql",
		SourceEnvUUID: "uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = sd.AddOffer(crossmodel.ServiceOffer{
		ServiceURL:    "local:/u/me/service",
		ServiceName:   "another",
		SourceEnvUUID: "uuid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add service offer "another" at "local:/u/me/service": service offer already exists`)
}

func (s *remoteServiceSuite) TestAddServiceOfferDuplicateAddedAfterInitial(c *gc.C) {
	// Check that a record with a URL conflict cannot be added if
	// there is no conflict initially but a record is added
	// before the transaction is run.
	sd := state.NewServiceDirectory(s.State)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := sd.AddOffer(crossmodel.ServiceOffer{
			ServiceURL:    "local:/u/me/service",
			ServiceName:   "mysql",
			SourceEnvUUID: "uuid",
		})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err := sd.AddOffer(crossmodel.ServiceOffer{
		ServiceURL:    "local:/u/me/service",
		ServiceName:   "mysql",
		SourceEnvUUID: "uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, gc.ErrorMatches, `cannot add service offer "another": service offer already exists`)
}

func (s *serviceDirectorySuite) TestUpdateServiceOfferUUIDRequired(c *gc.C) {
	sd := state.NewServiceDirectory(s.State)
	err := sd.UpdateOffer(crossmodel.ServiceOffer{
		ServiceURL:  "local:/u/me/service",
		ServiceName: "mysql",
	})
	c.Assert(err, gc.ErrorMatches, `cannot update service offer "mysql": missing source environment UUID`)
}

func (s *serviceDirectorySuite) TestUpdateServiceOfferNotFound(c *gc.C) {
	sd := state.NewServiceDirectory(s.State)
	err := sd.UpdateOffer(crossmodel.ServiceOffer{
		ServiceURL:    "local:/u/me/service",
		ServiceName:   "mysql",
		SourceEnvUUID: "uuid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot update service offer "mysql": service offer "local:/u/me/service" not found`)
}

func (s *remoteServiceSuite) TestUpdateServiceOfferRemovedAfterInitial(c *gc.C) {
	// Check that a record with a URL conflict cannot be added if
	// there is no conflict initially but a record is added
	// before the transaction is run.
	sd := state.NewServiceDirectory(s.State)
	err := sd.AddOffer(crossmodel.ServiceOffer{
		ServiceURL:    "local:/u/me/service",
		ServiceName:   "mysql",
		SourceEnvUUID: "uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := sd.Remove("local:/u/me/service")
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err = sd.UpdateOffer(crossmodel.ServiceOffer{
		ServiceURL:    "local:/u/me/service",
		ServiceName:   "mysql",
		SourceEnvUUID: "uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, gc.ErrorMatches, `cannot add service offer "another": service offer already exists`)
}
