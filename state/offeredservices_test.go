// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
)

type offeredServicesSuite struct {
	ConnSuite
}

var _ = gc.Suite(&offeredServicesSuite{})

func (s *offeredServicesSuite) createDefaultOffer(c *gc.C) crossmodel.OfferedService {
	eps := []string{"db", "db-admin"}
	offered := state.NewOfferedServices(s.State)
	offer := crossmodel.OfferedService{
		ServiceURL:  "local:/u/me/service",
		ServiceName: "mysql",
		Endpoints:   eps,
	}
	err := offered.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)
	return offer
}

func (s *offeredServicesSuite) TestRemove(c *gc.C) {
	offer := s.createDefaultOffer(c)
	offered := state.NewOfferedServices(s.State)
	err := offered.RemoveOffer(offer.ServiceName, offer.ServiceURL)
	c.Assert(err, jc.ErrorIsNil)
	n, err := state.OfferedServicesCount(s.State, offer.ServiceName, offer.ServiceURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 0)
}

func (s *offeredServicesSuite) TestAddServiceOffer(c *gc.C) {
	eps := []string{"mysql", "mysql-root"}
	offered := state.NewOfferedServices(s.State)
	offer := crossmodel.OfferedService{
		ServiceURL:  "local:/u/me/service",
		ServiceName: "mysql",
		Endpoints:   eps,
	}
	err := offered.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)
	n, err := state.OfferedServicesCount(s.State, offer.ServiceName, offer.ServiceURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
	offers, err := offered.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *offeredServicesSuite) TestListOffersNone(c *gc.C) {
	sd := state.NewOfferedServices(s.State)
	offers, err := sd.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 0)
}

func (s *offeredServicesSuite) createOffedService(c *gc.C, name string) crossmodel.OfferedService {
	eps := []string{name + "-interface", name + "-interface2"}
	offers := state.NewOfferedServices(s.State)
	offer := crossmodel.OfferedService{
		ServiceURL:  "local:/u/me/" + name,
		ServiceName: name,
		Endpoints:   eps,
	}
	err := offers.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)
	return offer
}

func (s *offeredServicesSuite) TestListOffersAll(c *gc.C) {
	offeredServices := state.NewOfferedServices(s.State)
	offer := s.createDefaultOffer(c)
	offers, err := offeredServices.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *offeredServicesSuite) TestListOffersOneFilter(c *gc.C) {
	offeredServices := state.NewOfferedServices(s.State)
	offer := s.createOffedService(c, "offer1")
	s.createOffedService(c, "offer2")
	s.createOffedService(c, "offer3")
	offers, err := offeredServices.ListOffers(crossmodel.OfferedServiceFilter{
		ServiceURL:  "local:/u/me/offer1",
		ServiceName: "offer1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *offeredServicesSuite) TestListOffersManyFilters(c *gc.C) {
	offeredServices := state.NewOfferedServices(s.State)
	offer := s.createOffedService(c, "offer1")
	offer2 := s.createOffedService(c, "offer2")
	s.createOffedService(c, "offer3")
	offers, err := offeredServices.ListOffers(
		crossmodel.OfferedServiceFilter{
			ServiceURL:  "local:/u/me/offer1",
			ServiceName: "offer1",
		},
		crossmodel.OfferedServiceFilter{
			ServiceURL: "local:/u/me/offer2",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 2)
	c.Assert(offers, jc.DeepEquals, []crossmodel.OfferedService{offer, offer2})
}

func (s *offeredServicesSuite) TestListOffersFilterServiceURLRegexp(c *gc.C) {
	offeredServices := state.NewOfferedServices(s.State)
	s.createOffedService(c, "offer1")
	offer := s.createOffedService(c, "offer2")
	s.createOffedService(c, "offer3")
	offers, err := offeredServices.ListOffers(crossmodel.OfferedServiceFilter{
		ServiceURL: "me/offer2",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *offeredServicesSuite) TestRegisterOffer(c *gc.C) {
	offeredServices := state.NewOfferedServices(s.State)
	offer := s.createOffedService(c, "offer1")
	offer2 := s.createOffedService(c, "offer2")
	s.createOffedService(c, "offer3")
	err := offeredServices.RegisterOffer("offer3", "local:/u/me/offer3")
	c.Assert(err, jc.ErrorIsNil)
	offers, err := offeredServices.UnregisteredOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 2)
	c.Assert(offers, jc.DeepEquals, []crossmodel.OfferedService{offer, offer2})
}

func (s *offeredServicesSuite) TestUnregisteredOffers(c *gc.C) {
	offeredServices := state.NewOfferedServices(s.State)
	offer := s.createOffedService(c, "offer1")
	offer2 := s.createOffedService(c, "offer2")
	state.AddRegisteredOffer(c, s.State, "offer3")
	offers, err := offeredServices.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 3)
	offers, err = offeredServices.UnregisteredOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 2)
	c.Assert(offers, jc.DeepEquals, []crossmodel.OfferedService{offer, offer2})
}

func (s *offeredServicesSuite) TestAddServiceOfferDuplicate(c *gc.C) {
	offered := state.NewOfferedServices(s.State)
	err := offered.AddOffer(crossmodel.OfferedService{
		ServiceURL:  "local:/u/me/service",
		ServiceName: "mysql",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = offered.AddOffer(crossmodel.OfferedService{
		ServiceURL:  "local:/u/me/service",
		ServiceName: "mysql",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add service offer "mysql" at "local:/u/me/service": service offer already exists`)
}

func (s *offeredServicesSuite) TestAddServiceOfferDuplicateAddedAfterInitial(c *gc.C) {
	// Check that a record with a URL conflict cannot be added if
	// there is no conflict initially but a record is added
	// before the transaction is run.
	offers := state.NewOfferedServices(s.State)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := offers.AddOffer(crossmodel.OfferedService{
			ServiceURL:  "local:/u/me/service",
			ServiceName: "mysql",
		})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err := offers.AddOffer(crossmodel.OfferedService{
		ServiceURL:  "local:/u/me/service",
		ServiceName: "mysql",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add service offer "mysql" at "local:/u/me/service": service offer already exists`)
}

func (s *offeredServicesSuite) TestRegisterOfferDeleteAfterInitial(c *gc.C) {
	offeredServices := state.NewOfferedServices(s.State)
	offer := s.createOffedService(c, "offer1")
	defer state.SetBeforeHooks(c, s.State, func() {
		err := offeredServices.RemoveOffer(offer.ServiceName, offer.ServiceURL)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err := offeredServices.RegisterOffer(offer.ServiceName, offer.ServiceURL)
	c.Assert(err, gc.ErrorMatches, `.* service offer "offer1" at url "local:/u/me/offer1" not found`)
}
