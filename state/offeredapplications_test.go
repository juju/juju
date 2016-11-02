// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type offeredApplicationsSuite struct {
	ConnSuite
}

var _ = gc.Suite(&offeredApplicationsSuite{})

func (s *offeredApplicationsSuite) createDefaultOffer(c *gc.C) crossmodel.OfferedApplication {
	eps := map[string]string{"db": "db", "db-admin": "dbadmin"}
	offered := state.NewOfferedApplications(s.State)
	offer := crossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/me/service",
		ApplicationName: "mysql",
		CharmName:       "charm",
		Description:     "description",
		Endpoints:       eps,
		Registered:      true,
	}
	err := offered.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)
	return offer
}

func (s *offeredApplicationsSuite) TestRemove(c *gc.C) {
	offer := s.createDefaultOffer(c)
	offered := state.NewOfferedApplications(s.State)
	err := offered.RemoveOffer(offer.ApplicationURL)
	c.Assert(err, jc.ErrorIsNil)

	modelTag := names.NewModelTag(s.State.ModelUUID())
	anotherState, err := state.Open(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      s.State.ControllerTag(),
		ControllerModelTag: modelTag,
		MongoInfo:          testing.NewMongoInfo(),
		MongoDialOpts:      mongotest.DialOpts(),
	})
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	offered = state.NewOfferedApplications(anotherState)
	offers, err := offered.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *offeredApplicationsSuite) TestAddApplicationOffer(c *gc.C) {
	eps := map[string]string{"mysql": "mysql", "mysql-admin": "admin"}
	offered := state.NewOfferedApplications(s.State)
	offer := crossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/me/service",
		ApplicationName: "mysql",
		CharmName:       "charm",
		Description:     "description",
		Endpoints:       eps,
	}
	err := offered.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)

	modelTag := names.NewModelTag(s.State.ModelUUID())
	anotherState, err := state.Open(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      s.State.ControllerTag(),
		ControllerModelTag: modelTag,
		MongoInfo:          testing.NewMongoInfo(),
		MongoDialOpts:      mongotest.DialOpts(),
	})
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	offered = state.NewOfferedApplications(anotherState)
	offers, err := offered.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	offers, err = offered.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 1)
	// offers are always created as registered.
	offer.Registered = true
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *offeredApplicationsSuite) TestUpdateApplicationOffer(c *gc.C) {
	eps := map[string]string{"db": "db"}
	offered := state.NewOfferedApplications(s.State)
	offer := crossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/me/service",
		ApplicationName: "mysql",
		Endpoints:       eps,
	}
	err := offered.AddOffer(offer)
	c.Assert(err, jc.ErrorIsNil)

	err = offered.UpdateOffer("local:/u/me/service", map[string]string{"db": "db", "db-admin": "admin"})
	c.Assert(err, jc.ErrorIsNil)

	modelTag := names.NewModelTag(s.State.ModelUUID())
	anotherState, err := state.Open(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      s.State.ControllerTag(),
		ControllerModelTag: modelTag,
		MongoInfo:          testing.NewMongoInfo(),
		MongoDialOpts:      mongotest.DialOpts(),
	})
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	offered = state.NewOfferedApplications(anotherState)
	offers, err := offered.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	offers, err = offered.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 1)

	expectedOffer := crossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/me/service",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"db": "db", "db-admin": "admin"},
		Registered:      true,
	}
	c.Assert(offers[0], jc.DeepEquals, expectedOffer)
}

func (s *offeredApplicationsSuite) TestListOffersNone(c *gc.C) {
	sd := state.NewOfferedApplications(s.State)
	offers, err := sd.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 0)
}

func (s *offeredApplicationsSuite) createOffedService(c *gc.C, name string) crossmodel.OfferedApplication {
	eps := map[string]string{name + "-interface": "ep1", name + "-interface2": "ep2"}
	offers := state.NewOfferedApplications(s.State)
	offer := crossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/me/" + name,
		CharmName:       name + "charm",
		ApplicationName: name,
		Endpoints:       eps,
	}
	err := offers.AddOffer(offer)
	// offers are always created as registered.
	offer.Registered = true
	c.Assert(err, jc.ErrorIsNil)
	return offer
}

func (s *offeredApplicationsSuite) TestListOffersAll(c *gc.C) {
	offeredApplications := state.NewOfferedApplications(s.State)
	offer := s.createDefaultOffer(c)
	offers, err := offeredApplications.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *offeredApplicationsSuite) TestListOffersServiceNameFilter(c *gc.C) {
	offeredApplications := state.NewOfferedApplications(s.State)
	offer := s.createOffedService(c, "offer1")
	s.createOffedService(c, "offer2")
	s.createOffedService(c, "offer3")
	offers, err := offeredApplications.ListOffers(crossmodel.OfferedApplicationFilter{
		ApplicationURL:  "local:/u/me/offer1",
		ApplicationName: "offer1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *offeredApplicationsSuite) TestListOffersCharmNameFilter(c *gc.C) {
	offeredApplications := state.NewOfferedApplications(s.State)
	offer := s.createOffedService(c, "offer1")
	s.createOffedService(c, "offer2")
	s.createOffedService(c, "offer3")
	offers, err := offeredApplications.ListOffers(crossmodel.OfferedApplicationFilter{
		ApplicationURL: "local:/u/me/offer1",
		CharmName:      "offer1charm",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *offeredApplicationsSuite) TestListOffersManyFilters(c *gc.C) {
	offeredApplications := state.NewOfferedApplications(s.State)
	offer := s.createOffedService(c, "offer1")
	offer2 := s.createOffedService(c, "offer2")
	s.createOffedService(c, "offer3")
	offers, err := offeredApplications.ListOffers(
		crossmodel.OfferedApplicationFilter{
			ApplicationURL:  "local:/u/me/offer1",
			ApplicationName: "offer1",
		},
		crossmodel.OfferedApplicationFilter{
			ApplicationURL: "local:/u/me/offer2",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 2)
	c.Assert(offers, jc.DeepEquals, []crossmodel.OfferedApplication{offer, offer2})
}

func (s *offeredApplicationsSuite) TestListOffersFilterApplicationURLRegexp(c *gc.C) {
	offeredApplications := state.NewOfferedApplications(s.State)
	s.createOffedService(c, "offer1")
	offer := s.createOffedService(c, "offer2")
	s.createOffedService(c, "offer3")
	offers, err := offeredApplications.ListOffers(crossmodel.OfferedApplicationFilter{
		ApplicationURL: "me/offer2",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 1)
	c.Assert(offers[0], jc.DeepEquals, offer)
}

func (s *offeredApplicationsSuite) TestSetOfferRegistered(c *gc.C) {
	offeredApplications := state.NewOfferedApplications(s.State)
	offer := s.createOffedService(c, "offer1")
	offer2 := s.createOffedService(c, "offer2")
	s.createOffedService(c, "offer3")
	err := offeredApplications.SetOfferRegistered("local:/u/me/offer3", false)
	c.Assert(err, jc.ErrorIsNil)
	offers, err := offeredApplications.ListOffers(crossmodel.RegisteredFilter(true))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(offers), gc.Equals, 2)
	c.Assert(offers, jc.DeepEquals, []crossmodel.OfferedApplication{offer, offer2})
}

func (s *offeredApplicationsSuite) TestUpdateApplicationOfferNotFound(c *gc.C) {
	offered := state.NewOfferedApplications(s.State)
	err := offered.UpdateOffer("local:/u/me/service", map[string]string{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, `.* application offer at "local:/u/me/service" not found`)
}

func (s *offeredApplicationsSuite) TestUpdateApplicationOfferNoEndpoints(c *gc.C) {
	offered := state.NewOfferedApplications(s.State)
	err := offered.UpdateOffer("local:/u/me/service", nil)
	c.Assert(err, gc.ErrorMatches, ".* no endpoints specified")
}

func (s *offeredApplicationsSuite) TestAddApplicationOfferDuplicate(c *gc.C) {
	offered := state.NewOfferedApplications(s.State)
	err := offered.AddOffer(crossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/me/service",
		ApplicationName: "mysql",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = offered.AddOffer(crossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/me/service",
		ApplicationName: "mysql",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application offer "mysql" at "local:/u/me/service": application offer already exists`)
}

func (s *offeredApplicationsSuite) TestAddApplicationOfferDuplicateAddedAfterInitial(c *gc.C) {
	// Check that a record with a URL conflict cannot be added if
	// there is no conflict initially but a record is added
	// before the transaction is run.
	offers := state.NewOfferedApplications(s.State)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := offers.AddOffer(crossmodel.OfferedApplication{
			ApplicationURL:  "local:/u/me/service",
			ApplicationName: "mysql",
		})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err := offers.AddOffer(crossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/me/service",
		ApplicationName: "mysql",
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application offer "mysql" at "local:/u/me/service": application offer already exists`)
}

func (s *offeredApplicationsSuite) TestRegisterOfferDeleteAfterInitial(c *gc.C) {
	offeredApplications := state.NewOfferedApplications(s.State)
	offer := s.createOffedService(c, "offer1")
	defer state.SetBeforeHooks(c, s.State, func() {
		err := offeredApplications.RemoveOffer(offer.ApplicationURL)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err := offeredApplications.SetOfferRegistered(offer.ApplicationURL, false)
	c.Assert(err, gc.ErrorMatches, `.* application offer at "local:/u/me/offer1" not found`)
}

func (s *offeredApplicationsSuite) TestUpdateOfferDeleteAfterInitial(c *gc.C) {
	offeredApplications := state.NewOfferedApplications(s.State)
	offer := s.createOffedService(c, "offer1")
	defer state.SetBeforeHooks(c, s.State, func() {
		err := offeredApplications.RemoveOffer(offer.ApplicationURL)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	err := offeredApplications.UpdateOffer(offer.ApplicationURL, map[string]string{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, `.* application offer at "local:/u/me/offer1" not found`)
}
