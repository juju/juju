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
	model "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/testing"
)

type crossmodelMockSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&crossmodelMockSuite{})

func (s *crossmodelMockSuite) TestOffer(c *gc.C) {
	service := "shared"
	endPointA := "endPointA"
	endPointB := "endPointB"
	url := "url"
	user1 := "user1"
	user2 := "user2"
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

			args, ok := a.(params.ApplicationOffersParams)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Offers, gc.HasLen, 1)

			offer := args.Offers[0]
			c.Assert(offer.ApplicationName, gc.DeepEquals, service)
			c.Assert(offer.Endpoints, jc.SameContents, []string{endPointA, endPointB})
			c.Assert(offer.ApplicationURL, gc.DeepEquals, url)
			c.Assert(offer.ApplicationDescription, gc.DeepEquals, desc)
			c.Assert(offer.AllowedUserTags, jc.SameContents, []string{user1, user2})

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
	results, err := client.Offer(service, []string{endPointA, endPointB}, url, []string{user1, user2}, desc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results, jc.DeepEquals,
		[]params.ErrorResult{
			params.ErrorResult{},
			params.ErrorResult{Error: common.ServerError(errors.New(msg))},
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
	results, err := client.Offer("", nil, "", nil, "")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}

func (s *crossmodelMockSuite) TestShow(c *gc.C) {
	url := "local:/u/fred/db2"

	desc := "IBM DB2 Express Server Edition is an entry level database system"
	endpoints := []params.RemoteEndpoint{
		params.RemoteEndpoint{Name: "db2", Interface: "db2", Role: charm.RoleProvider},
		params.RemoteEndpoint{Name: "log", Interface: "http", Role: charm.RoleRequirer},
	}
	serviceTag := "service-hosted-db2"

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
						ApplicationName:        serviceTag,
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
		ApplicationName:        serviceTag})
}

func (s *crossmodelMockSuite) TestShowURLError(c *gc.C) {
	url := "local:/u/fred/db2"
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
	url := "local:/u/fred/db2"

	desc := "IBM DB2 Express Server Edition is an entry level database system"
	endpoints := []params.RemoteEndpoint{
		params.RemoteEndpoint{Name: "db2", Interface: "db2", Role: charm.RoleProvider},
		params.RemoteEndpoint{Name: "log", Interface: "http", Role: charm.RoleRequirer},
	}
	serviceTag := "service-hosted-db2"

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
						ApplicationName:        serviceTag,
					}},
					{Result: params.ApplicationOffer{
						ApplicationDescription: desc,
						Endpoints:              endpoints,
						ApplicationName:        serviceTag,
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

func (s *crossmodelMockSuite) TestShowFacadeCallError(c *gc.C) {
	url := "local:/u/fred/db2"
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
	found, err := client.ApplicationOffer(url)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.DeepEquals, params.ApplicationOffer{})
}

func (s *crossmodelMockSuite) TestList(c *gc.C) {
	directoryName := "local"
	charmName := "db2"
	serviceName := fmt.Sprintf("hosted-%s", charmName)
	url := fmt.Sprintf("%s:/u/fred/%s", directoryName, serviceName)
	endpoints := []params.RemoteEndpoint{{Name: "endPointA"}}
	relations := []charm.Relation{{Name: "endPointA"}}
	count := 23

	msg := "item fail"
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListOffers")

			called = true
			args, ok := a.(params.OfferedApplicationFilters)
			c.Assert(ok, jc.IsTrue)
			//TODO (anastasiamac 2015-11-18) To add proper check once filters are implemented
			c.Assert(args.Filters, gc.HasLen, 1)

			if results, ok := result.(*params.ListOffersResults); ok {

				validItem := params.OfferedApplicationDetails{
					ApplicationURL:  url,
					ApplicationName: serviceName,
					CharmName:       charmName,
					UsersCount:      count,
					Endpoints:       endpoints,
				}

				validDir := params.ListOffersFilterResults{
					Result: []params.OfferedApplicationDetailsResult{
						{Result: &validItem},
						{Error: common.ServerError(errors.New(msg))},
					}}

				results.Results = []params.ListOffersFilterResults{validDir}
			}

			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	results, err := client.ListOffers(model.OfferedApplicationFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, []model.OfferedApplicationDetailsResult{
		{Result: &model.OfferedApplicationDetails{
			ApplicationName: serviceName,
			ApplicationURL:  url,
			CharmName:       charmName,
			ConnectedCount:  count,
			Endpoints:       relations,
		}},
		{Error: common.ServerError(errors.New(msg))},
	})
}

func (s *crossmodelMockSuite) TestListMultipleResults(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListOffers")

			called = true
			if results, ok := result.(*params.ListOffersResults); ok {
				results.Results = []params.ListOffersFilterResults{{}, {}}
			}

			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	_, err := client.ListOffers(model.OfferedApplicationFilter{})
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*expected to find one result but found 2.*")
	c.Assert(called, jc.IsTrue)
}

func (s *crossmodelMockSuite) TestListDirectoryError(c *gc.C) {
	msg := "dir failure"
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListOffers")

			called = true
			if results, ok := result.(*params.ListOffersResults); ok {
				results.Results = []params.ListOffersFilterResults{
					{Error: common.ServerError(errors.New(msg))},
				}
			}

			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	_, err := client.ListOffers(model.OfferedApplicationFilter{})
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
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListOffers")

			return errors.New(msg)
		})
	client := crossmodel.NewClient(apiCaller)
	results, err := client.ListOffers(model.OfferedApplicationFilter{})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}

func (s *crossmodelMockSuite) TestFind(c *gc.C) {
	directoryName := "local"
	charmName := "db2"
	serviceName := fmt.Sprintf("hosted-%s", charmName)
	url := fmt.Sprintf("%s:/u/fred/%s", directoryName, serviceName)
	endpoints := []params.RemoteEndpoint{{Name: "endPointA"}}
	relations := []charm.Relation{{Name: "endPointA", Interface: "http"}}

	filter := model.ApplicationOfferFilter{
		ApplicationOffer: model.ApplicationOffer{
			ApplicationURL:  fmt.Sprintf("%s:/u/fred/%s", directoryName, serviceName),
			ApplicationName: fmt.Sprintf("hosted-%s", charmName),
			Endpoints:       relations,
		},
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
			args, ok := a.(params.OfferFilterParams)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Filters, gc.HasLen, 1)
			c.Assert(args.Filters[0].Directory, gc.Equals, "local")
			c.Assert(args.Filters[0].Filters, jc.DeepEquals, []params.OfferFilter{{
				ApplicationURL:  filter.ApplicationURL,
				ApplicationName: filter.ApplicationName,
				Endpoints: []params.EndpointFilterAttributes{{
					Name:      "endPointA",
					Interface: "http",
				}},
			}})

			if results, ok := result.(*params.FindApplicationOffersResults); ok {
				offer := params.ApplicationOffer{
					ApplicationURL:  url,
					ApplicationName: serviceName,
					Endpoints:       endpoints,
				}
				results.Results = []params.ApplicationOfferResults{{
					Offers: []params.ApplicationOffer{offer},
				}}
			}

			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	results, err := client.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, []params.ApplicationOffer{{
		ApplicationName: serviceName,
		ApplicationURL:  url,
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

func (s *crossmodelMockSuite) TestFindMultipleResults(c *gc.C) {
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
			if results, ok := result.(*params.FindApplicationOffersResults); ok {
				results.Results = []params.ApplicationOfferResults{{}, {}}
			}

			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	filter := model.ApplicationOfferFilter{
		ApplicationOffer: model.ApplicationOffer{ApplicationURL: "local:"},
	}
	_, err := client.FindApplicationOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*expected to find one result but found 2.*")
	c.Assert(called, jc.IsTrue)
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
			if results, ok := result.(*params.FindApplicationOffersResults); ok {
				results.Results = []params.ApplicationOfferResults{{
					Error: common.ServerError(errors.New(msg)),
				}}
			}

			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	filter := model.ApplicationOfferFilter{
		ApplicationOffer: model.ApplicationOffer{ApplicationURL: "local:"},
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
	filter := model.ApplicationOfferFilter{
		ApplicationOffer: model.ApplicationOffer{ApplicationURL: "local:"},
	}
	results, err := client.FindApplicationOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}
