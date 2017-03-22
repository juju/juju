// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api/applicationoffers"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/testing"
)

type crossmodelMockSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&crossmodelMockSuite{})

func (s *crossmodelMockSuite) TestOffer(c *gc.C) {
	application := "shared"
	endPointA := "endPointA"
	endPointB := "endPointB"
	offer := "offer"
	desc := "desc"

	msg := "fail"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Offer")

			args, ok := a.(params.AddApplicationOffers)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Offers, gc.HasLen, 1)

			offer := args.Offers[0]
			c.Assert(offer.ApplicationName, gc.DeepEquals, application)
			c.Assert(offer.Endpoints, jc.DeepEquals, map[string]string{endPointA: endPointA, endPointB: endPointB})
			c.Assert(offer.OfferName, gc.Equals, offer.OfferName)
			c.Assert(offer.ApplicationDescription, gc.DeepEquals, desc)

			if results, ok := result.(*params.ErrorResults); ok {
				all := make([]params.ErrorResult, len(args.Offers))
				// add one error to make sure it's catered for.
				all = append(all, params.ErrorResult{
					Error: common.ServerError(errors.New(msg))})
				results.Results = all
			}

			return nil
		})

	client := applicationoffers.NewClient(apiCaller)
	results, err := client.Offer(application, []string{endPointA, endPointB}, offer, desc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results, jc.DeepEquals,
		[]params.ErrorResult{
			{},
			{Error: common.ServerError(errors.New(msg))},
		})
}

func (s *crossmodelMockSuite) TestOfferFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Offer")

			return errors.New(msg)
		})
	client := applicationoffers.NewClient(apiCaller)
	results, err := client.Offer("", nil, "", "")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}

func (s *crossmodelMockSuite) TestList(c *gc.C) {
	offerName := "hosted-db2"
	url := fmt.Sprintf("fred/model.%s", offerName)
	endpoints := []params.RemoteEndpoint{{Name: "endPointA"}}
	relations := []jujucrossmodel.EndpointFilterTerm{{Name: "endPointA", Interface: "http"}}

	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: offerName,
		Endpoints: relations,
	}

	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListApplicationOffers")

			called = true
			args, ok := a.(params.OfferFilters)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Filters, gc.HasLen, 1)
			c.Assert(args.Filters[0], jc.DeepEquals, params.OfferFilter{
				OfferName:       filter.OfferName,
				ApplicationName: filter.ApplicationName,
				Endpoints: []params.EndpointFilterAttributes{{
					Name:      "endPointA",
					Interface: "http",
				}},
			})

			if results, ok := result.(*params.ListApplicationOffersResults); ok {
				offer := params.ApplicationOffer{
					OfferURL:  url,
					OfferName: offerName,
					Endpoints: endpoints,
				}
				results.Results = []params.ApplicationOfferDetails{{
					ApplicationOffer: offer,
					ApplicationName:  "db2-app",
					CharmName:        "db2",
					ConnectedCount:   3,
				}}
			}
			return nil
		})

	client := applicationoffers.NewClient(apiCaller)
	results, err := client.ListOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, []jujucrossmodel.ApplicationOfferDetailsResult{{
		Result: &jujucrossmodel.ApplicationOfferDetails{
			OfferURL:        url,
			OfferName:       offerName,
			Endpoints:       []charm.Relation{{Name: "endPointA"}},
			ApplicationName: "db2-app",
			CharmName:       "db2",
			ConnectedCount:  3,
		}}})
}

func (s *crossmodelMockSuite) TestListError(c *gc.C) {
	msg := "find failure"
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListApplicationOffers")

			called = true
			return errors.New(msg)
		})

	client := applicationoffers.NewClient(apiCaller)
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}
	_, err := client.ListOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}

func (s *crossmodelMockSuite) TestListFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListApplicationOffers")

			return errors.New(msg)
		})
	client := applicationoffers.NewClient(apiCaller)
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}
	results, err := client.ListOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}
