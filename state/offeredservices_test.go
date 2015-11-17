// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type offeredServicesSuite struct {
	ConnSuite
}

var _ = gc.Suite(&offeredServicesSuite{})

func (s *offeredServicesSuite) createDefaultOffer(c *gc.C) crossmodel.OfferedService {
	eps := map[string]string{"db": "db", "db-admin": "dbadmin"}
	offered := state.NewOfferedServices(s.State)
	offer := crossmodel.OfferedService{
		ServiceURL:  "local:/u/me/service",
		ServiceName: "mysql",
		Endpoints:   eps,
		Registered:  true,
	}
	err := offered.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)
	return offer
}

func (s *offeredServicesSuite) TestRemove(c *gc.C) {
	offer := s.createDefaultOffer(c)
	offered := state.NewOfferedServices(s.State)
	err := offered.RemoveOffer(offer.ServiceURL)
	c.Assert(err, jc.ErrorIsNil)

	envTag := names.NewEnvironTag(s.State.EnvironUUID())
	anotherState, err := state.Open(envTag, testing.NewMongoInfo(), testing.NewDialOpts(), state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	offered = state.NewOfferedServices(anotherState)
	offers, err := offered.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *offeredServicesSuite) TestAddServiceOffer(c *gc.C) {
	eps := map[string]string{"mysql": "mysql", "mysql-admin": "admin"}
	offered := state.NewOfferedServices(s.State)
	offer := crossmodel.OfferedService{
		ServiceURL:  "local:/u/me/service",
		ServiceName: "mysql",
		Endpoints:   eps,
	}
	err := offered.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)

	envTag := names.NewEnvironTag(s.State.EnvironUUID())
	anotherState, err := state.Open(envTag, testing.NewMongoInfo(), testing.NewDialOpts(), state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	offered = state.NewOfferedServices(anotherState)
	offers, err := offered.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	offers, err = offered.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 1)
	// offers are always created as registered.
	offer.Registered = true
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *offeredServicesSuite) TestUpdateServiceOffer(c *gc.C) {
	eps := map[string]string{"db": "db"}
	offered := state.NewOfferedServices(s.State)
	offer := crossmodel.OfferedService{
		ServiceURL:  "local:/u/me/service",
		ServiceName: "mysql",
		Endpoints:   eps,
	}
	err := offered.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)

	err = offered.UpdateOffer("local:/u/me/service", map[string]string{"db": "db", "db-admin": "admin"})
	c.Assert(err, jc.ErrorIsNil)

	envTag := names.NewEnvironTag(s.State.EnvironUUID())
	anotherState, err := state.Open(envTag, testing.NewMongoInfo(), testing.NewDialOpts(), state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	offered = state.NewOfferedServices(anotherState)
	offers, err := offered.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	offers, err = offered.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 1)

	expectedOffer := crossmodel.OfferedService{
		ServiceURL:  "local:/u/me/service",
		ServiceName: "mysql",
		Endpoints:   map[string]string{"db": "db", "db-admin": "admin"},
		Registered:  true,
	}
	c.Assert(offers[0], jc.DeepEquals, expectedOffer)
}

func (s *offeredServicesSuite) TestListOffersNone(c *gc.C) {
	sd := state.NewOfferedServices(s.State)
	offers, err := sd.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 0)
}

func (s *offeredServicesSuite) createOffedService(c *gc.C, name string) crossmodel.OfferedService {
	eps := map[string]string{name + "-interface": "ep1", name + "-interface2": "ep2"}
	offers := state.NewOfferedServices(s.State)
	offer := crossmodel.OfferedService{
		ServiceURL:  "local:/u/me/" + name,
		ServiceName: name,
		Endpoints:   eps,
	}
	err := offers.AddOffer(offer)
	// offers are always created as registered.
	offer.Registered = true
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

func (s *offeredServicesSuite) TestSetOfferRegistered(c *gc.C) {
	offeredServices := state.NewOfferedServices(s.State)
	offer := s.createOffedService(c, "offer1")
	offer2 := s.createOffedService(c, "offer2")
	s.createOffedService(c, "offer3")
	err := offeredServices.SetOfferRegistered("local:/u/me/offer3", false)
	c.Assert(err, jc.ErrorIsNil)
	offers, err := offeredServices.ListOffersByRegisteredState(true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 2)
	c.Assert(offers, jc.DeepEquals, []crossmodel.OfferedService{offer, offer2})
}

func (s *offeredServicesSuite) TestUpdateServiceOfferNotFound(c *gc.C) {
	offered := state.NewOfferedServices(s.State)
	err := offered.UpdateOffer("local:/u/me/service", map[string]string{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, `.* service offer at "local:/u/me/service" not found`)
}

func (s *offeredServicesSuite) TestUpdateServiceOfferNoEndpoints(c *gc.C) {
	offered := state.NewOfferedServices(s.State)
	err := offered.UpdateOffer("local:/u/me/service", nil)
	c.Assert(err, gc.ErrorMatches, ".* no endpoints specified")
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
		err := offeredServices.RemoveOffer(offer.ServiceURL)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err := offeredServices.SetOfferRegistered(offer.ServiceURL, false)
	c.Assert(err, gc.ErrorMatches, `.* service offer at "local:/u/me/offer1" not found`)
}

func (s *offeredServicesSuite) TestUpdateOfferDeleteAfterInitial(c *gc.C) {
	offeredServices := state.NewOfferedServices(s.State)
	offer := s.createOffedService(c, "offer1")
	defer state.SetBeforeHooks(c, s.State, func() {
		err := offeredServices.RemoveOffer(offer.ServiceURL)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err := offeredServices.UpdateOffer(offer.ServiceURL, map[string]string{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, `.* service offer at "local:/u/me/offer1" not found`)
}
