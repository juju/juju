// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/crossmodel"
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
			c.Check(objType, gc.Equals, "CrossModelRelations")
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

	client := crossmodel.NewClient(apiCaller)
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
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Offer")

			return errors.New(msg)
		})
	client := crossmodel.NewClient(apiCaller)
	results, err := client.Offer("", nil, "", "")
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
	applicationTag := "application-hosted-db2"

	called := false

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true

			c.Check(objType, gc.Equals, "CrossModelRelations")
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
						ApplicationName:        applicationTag,
					}},
				}
			}
			return nil
		})
	client := crossmodel.NewClient(apiCaller)
	found, err := client.ApplicationOffer(url)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(called, jc.IsTrue)
	c.Assert(found, gc.DeepEquals, params.ApplicationOffer{
		ApplicationDescription: desc,
		Endpoints:              endpoints,
		ApplicationName:        applicationTag})
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

			c.Check(objType, gc.Equals, "CrossModelRelations")
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
	client := crossmodel.NewClient(apiCaller)
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
	applicationTag := "application-hosted-db2"

	called := false

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true

			c.Check(objType, gc.Equals, "CrossModelRelations")
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
						ApplicationName:        applicationTag,
					}},
					{Result: params.ApplicationOffer{
						ApplicationDescription: desc,
						Endpoints:              endpoints,
						ApplicationName:        applicationTag,
					}}}
			}
			return nil
		})
	client := crossmodel.NewClient(apiCaller)
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
	client := crossmodel.NewClient(apiCaller)
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
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ApplicationOffers")

			return errors.New(msg)
		})
	client := crossmodel.NewClient(apiCaller)
	found, err := client.ApplicationOffer("fred/model.db2")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.DeepEquals, params.ApplicationOffer{})
}

func (s *crossmodelMockSuite) TestFind(c *gc.C) {
	charmName := "db2"
	offerName := fmt.Sprintf("hosted-%s", charmName)
	url := fmt.Sprintf("fred/model.%s", offerName)
	endpoints := []params.RemoteEndpoint{{Name: "endPointA"}}
	relations := []jujucrossmodel.EndpointFilterTerm{{Name: "endPointA", Interface: "http"}}

	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName:       charmName,
		ApplicationName: fmt.Sprintf("hosted-%s", charmName),
		Endpoints:       relations,
	}

	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "FindApplicationOffers")

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

			if results, ok := result.(*params.FindApplicationOffersResults); ok {
				offer := params.ApplicationOffer{
					OfferURL:        url,
					OfferName:       offerName,
					ApplicationName: charmName,
					Endpoints:       endpoints,
				}
				results.Results = []params.ApplicationOffer{offer}
			}
			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	results, err := client.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, []params.ApplicationOffer{{
		OfferURL:        url,
		OfferName:       offerName,
		ApplicationName: charmName,
		Endpoints:       endpoints,
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

	client := crossmodel.NewClient(apiCaller)
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
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "FindApplicationOffers")

			called = true
			return errors.New(msg)
		})

	client := crossmodel.NewClient(apiCaller)
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
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "FindApplicationOffers")

			return errors.New(msg)
		})
	client := crossmodel.NewClient(apiCaller)
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}
	results, err := client.FindApplicationOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}
