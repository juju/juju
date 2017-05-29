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
			c.Assert(offer.ModelTag, gc.Equals, "model-uuid")
			c.Assert(offer.ApplicationName, gc.Equals, application)
			c.Assert(offer.Endpoints, jc.DeepEquals, map[string]string{endPointA: endPointA, endPointB: endPointB})
			c.Assert(offer.OfferName, gc.Equals, offer.OfferName)
			c.Assert(offer.ApplicationDescription, gc.Equals, desc)

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
	results, err := client.Offer("uuid", application, []string{endPointA, endPointB}, offer, desc)
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
	results, err := client.Offer("", "", nil, "", "")
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

func (s *crossmodelMockSuite) TestShow(c *gc.C) {
	url := "fred/model.db2"

	desc := "IBM DB2 Express Server Edition is an entry level database system"
	endpoints := []params.RemoteEndpoint{
		{Name: "db2", Interface: "db2", Role: charm.RoleProvider},
		{Name: "log", Interface: "http", Role: charm.RoleRequirer},
	}
	offerName := "hosted-db2"
	access := "consume"

	called := false

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true

			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ApplicationOffers")

			args, ok := a.(params.ApplicationURLs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.ApplicationURLs, gc.DeepEquals, []string{url})

			if points, ok := result.(*params.ApplicationOffersResults); ok {
				points.Results = []params.ApplicationOfferResult{
					{Result: params.ApplicationOffer{
						ApplicationDescription: desc,
						Endpoints:              endpoints,
						OfferURL:               url,
						OfferName:              offerName,
						Access:                 access,
					}},
				}
			}
			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	found, err := client.ApplicationOffer(url)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(called, jc.IsTrue)
	c.Assert(found, gc.DeepEquals, params.ApplicationOffer{
		ApplicationDescription: desc,
		Endpoints:              endpoints,
		OfferURL:               url,
		OfferName:              offerName,
		Access:                 access})
}

func (s *crossmodelMockSuite) TestShowURLError(c *gc.C) {
	url := "fred/model.db2"
	msg := "facade failure"

	called := false

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true

			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ApplicationOffers")

			args, ok := a.(params.ApplicationURLs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.ApplicationURLs, gc.DeepEquals, []string{url})

			if points, ok := result.(*params.ApplicationOffersResults); ok {
				points.Results = []params.ApplicationOfferResult{
					{Error: common.ServerError(errors.New(msg))}}
			}
			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	found, err := client.ApplicationOffer(url)

	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.DeepEquals, params.ApplicationOffer{})
	c.Assert(called, jc.IsTrue)
}

func (s *crossmodelMockSuite) TestShowMultiple(c *gc.C) {
	url := "fred/model.db2"

	desc := "IBM DB2 Express Server Edition is an entry level database system"
	endpoints := []params.RemoteEndpoint{
		{Name: "db2", Interface: "db2", Role: charm.RoleProvider},
		{Name: "log", Interface: "http", Role: charm.RoleRequirer},
	}
	offerName := "hosted-db2"
	access := "consume"

	called := false

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true

			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ApplicationOffers")

			args, ok := a.(params.ApplicationURLs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.ApplicationURLs, gc.DeepEquals, []string{url})

			if points, ok := result.(*params.ApplicationOffersResults); ok {
				points.Results = []params.ApplicationOfferResult{
					{Result: params.ApplicationOffer{
						ApplicationDescription: desc,
						Endpoints:              endpoints,
						OfferURL:               url,
						OfferName:              offerName,
						Access:                 access,
					}},
					{Result: params.ApplicationOffer{
						ApplicationDescription: desc,
						Endpoints:              endpoints,
						OfferURL:               url,
						OfferName:              offerName,
						Access:                 access,
					}}}
			}
			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	found, err := client.ApplicationOffer(url)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`expected to find one result for url %q but found 2`, url))
	c.Assert(found, gc.DeepEquals, params.ApplicationOffer{})

	c.Assert(called, jc.IsTrue)
}

func (s *crossmodelMockSuite) TestShowNonLocal(c *gc.C) {
	url := "jaas:fred/model.db2"
	msg := "query for non-local application offers not supported"

	called := false

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	found, err := client.ApplicationOffer(url)

	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.DeepEquals, params.ApplicationOffer{})
	c.Assert(called, jc.IsFalse)
}

func (s *crossmodelMockSuite) TestShowFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ApplicationOffers")

			return errors.New(msg)
		})
	client := applicationoffers.NewClient(apiCaller)
	found, err := client.ApplicationOffer("fred/model.db2")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.DeepEquals, params.ApplicationOffer{})
}

func (s *crossmodelMockSuite) TestFind(c *gc.C) {
	offerName := "hosted-db2"
	ownerName := "owner"
	modelName := "model"
	access := "consume"
	url := fmt.Sprintf("fred/model.%s", offerName)
	endpoints := []params.RemoteEndpoint{{Name: "endPointA"}}
	relations := []jujucrossmodel.EndpointFilterTerm{{Name: "endPointA", Interface: "http"}}

	filter := jujucrossmodel.ApplicationOfferFilter{
		OwnerName: ownerName,
		ModelName: modelName,
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
			c.Check(request, gc.Equals, "FindApplicationOffers")

			called = true
			args, ok := a.(params.OfferFilters)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Filters, gc.HasLen, 1)
			c.Assert(args.Filters[0], jc.DeepEquals, params.OfferFilter{
				OwnerName:       filter.OwnerName,
				ModelName:       filter.ModelName,
				OfferName:       filter.OfferName,
				ApplicationName: filter.ApplicationName,
				Endpoints: []params.EndpointFilterAttributes{{
					Name:      "endPointA",
					Interface: "http",
				}},
			})

			if results, ok := result.(*params.FindApplicationOffersResults); ok {
				offer := params.ApplicationOffer{
					OfferURL:  url,
					OfferName: offerName,
					Endpoints: endpoints,
					Access:    access,
				}
				results.Results = []params.ApplicationOffer{offer}
			}
			return nil
		})

	client := applicationoffers.NewClient(apiCaller)
	results, err := client.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, []params.ApplicationOffer{{
		OfferURL:  url,
		OfferName: offerName,
		Endpoints: endpoints,
		Access:    access,
	}})
}

func (s *crossmodelMockSuite) TestFindNoFilter(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Fail()
			return nil
		})

	client := applicationoffers.NewClient(apiCaller)
	_, err := client.FindApplicationOffers()
	c.Assert(err, gc.ErrorMatches, "at least one filter must be specified")
}

func (s *crossmodelMockSuite) TestFindError(c *gc.C) {
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
			c.Check(request, gc.Equals, "FindApplicationOffers")

			called = true
			return errors.New(msg)
		})

	client := applicationoffers.NewClient(apiCaller)
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}
	_, err := client.FindApplicationOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}

func (s *crossmodelMockSuite) TestFindFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "FindApplicationOffers")

			return errors.New(msg)
		})
	client := applicationoffers.NewClient(apiCaller)
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}
	results, err := client.FindApplicationOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}
