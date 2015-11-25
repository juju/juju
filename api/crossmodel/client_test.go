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
	model "github.com/juju/juju/model/crossmodel"
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

			args, ok := a.(params.RemoteServiceOffers)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Offers, gc.HasLen, 1)

			offer := args.Offers[0]
			c.Assert(offer.ServiceName, gc.DeepEquals, service)
			c.Assert(offer.Endpoints, jc.SameContents, []string{endPointA, endPointB})
			c.Assert(offer.ServiceURL, gc.DeepEquals, url)
			c.Assert(offer.ServiceDescription, gc.DeepEquals, desc)
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
			c.Check(request, gc.Equals, "Show")

			args, ok := a.([]string)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args, gc.DeepEquals, []string{url})

			if points, ok := result.(*params.RemoteServiceResults); ok {
				points.Results = []params.RemoteServiceResult{
					{Result: params.ServiceOffer{
						ServiceDescription: desc,
						Endpoints:          endpoints,
						ServiceName:        serviceTag,
					}},
				}
			}
			return nil
		})
	client := crossmodel.NewClient(apiCaller)
	found, err := client.Show(url)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(called, jc.IsTrue)
	c.Assert(found, gc.DeepEquals, params.ServiceOffer{
		ServiceDescription: desc,
		Endpoints:          endpoints,
		ServiceName:        serviceTag})
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
			c.Check(request, gc.Equals, "Show")

			args, ok := a.([]string)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args, gc.DeepEquals, []string{url})

			if points, ok := result.(*params.RemoteServiceResults); ok {
				points.Results = []params.RemoteServiceResult{
					{Error: common.ServerError(errors.New(msg))}}
			}
			return nil
		})
	client := crossmodel.NewClient(apiCaller)
	found, err := client.Show(url)

	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.DeepEquals, params.ServiceOffer{})
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
			c.Check(request, gc.Equals, "Show")

			args, ok := a.([]string)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args, gc.DeepEquals, []string{url})

			if points, ok := result.(*params.RemoteServiceResults); ok {
				points.Results = []params.RemoteServiceResult{
					{Result: params.ServiceOffer{
						ServiceDescription: desc,
						Endpoints:          endpoints,
						ServiceName:        serviceTag,
					}},
					{Result: params.ServiceOffer{
						ServiceDescription: desc,
						Endpoints:          endpoints,
						ServiceName:        serviceTag,
					}}}
			}
			return nil
		})
	client := crossmodel.NewClient(apiCaller)
	found, err := client.Show(url)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`expected to find one result for url %q but found 2`, url))
	c.Assert(found, gc.DeepEquals, params.ServiceOffer{})

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
			c.Check(request, gc.Equals, "Show")

			return errors.New(msg)
		})
	client := crossmodel.NewClient(apiCaller)
	found, err := client.Show(url)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.DeepEquals, params.ServiceOffer{})
}

func (s *crossmodelMockSuite) TestList(c *gc.C) {
	directoryName := "local"
	charmName := "db2"
	serviceName := fmt.Sprintf("hosted-%s", charmName)
	url := fmt.Sprintf("%s:/u/fred/%s", directoryName, serviceName)
	endpoints := []charm.Relation{{Name: "endPointA"}}
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
			c.Check(request, gc.Equals, "List")

			called = true
			args, ok := a.(params.ListEndpointsFilters)
			c.Assert(ok, jc.IsTrue)
			//TODO (anastasiamac 2015-11-18) To add proper check once filters are implemented
			c.Assert(args.Filters, gc.HasLen, 1)

			if results, ok := result.(*params.ListEndpointsItemsResults); ok {

				validItem := params.ListEndpointsServiceItem{
					ServiceURL:  url,
					ServiceName: serviceName,
					CharmName:   charmName,
					UsersCount:  count,
					Endpoints:   endpoints,
				}

				validDir := params.ListEndpointsItemsResult{
					Result: []params.ListEndpointsServiceItemResult{
						{Result: &validItem},
						{Error: common.ServerError(errors.New(msg))},
					}}

				results.Results = []params.ListEndpointsItemsResult{validDir}
			}

			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	results, err := client.List(model.RemoteServiceFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, []model.ListEndpointsServiceResult{
		{Result: &model.ListEndpointsService{
			ServiceName:    serviceName,
			ServiceURL:     url,
			CharmName:      charmName,
			ConnectedCount: count,
			Endpoints:      endpoints,
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
			c.Check(request, gc.Equals, "List")

			called = true
			if results, ok := result.(*params.ListEndpointsItemsResults); ok {
				results.Results = []params.ListEndpointsItemsResult{{}, {}}
			}

			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	_, err := client.List(model.RemoteServiceFilter{})
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
			c.Check(request, gc.Equals, "List")

			called = true
			if results, ok := result.(*params.ListEndpointsItemsResults); ok {
				results.Results = []params.ListEndpointsItemsResult{
					{Error: common.ServerError(errors.New(msg))},
				}
			}

			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	_, err := client.List(model.RemoteServiceFilter{})
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
			c.Check(request, gc.Equals, "List")

			return errors.New(msg)
		})
	client := crossmodel.NewClient(apiCaller)
	results, err := client.List(model.RemoteServiceFilter{})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}

func (s *crossmodelMockSuite) TestAddRelation(c *gc.C) {

	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "AddRelation")

			called = true

			args, ok := a.(params.ListEndpointsFilters)
			c.Assert(ok, jc.IsTrue)
			//TODO (anastasiamac 2015-11-18) To add proper check once filters are implemented
			c.Assert(args.Filters, gc.HasLen, 1)

			if results, ok := result.(*params.ListEndpointsItemsResults); ok {

				validItem := params.ListEndpointsServiceItem{
					ServiceURL:  url,
					ServiceName: serviceName,
					CharmName:   charmName,
					UsersCount:  count,
					Endpoints:   endpoints,
				}

				validDir := params.ListEndpointsItemsResult{
					Result: []params.ListEndpointsServiceItemResult{
						{Result: &validItem},
						{Error: common.ServerError(errors.New(msg))},
					}}

				results.Results = []params.ListEndpointsItemsResult{validDir}
			}

			return nil
		})
	client := crossmodel.NewClient(apiCaller)
	results, err := client.AddRelation("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(results, gc.IsNil)
}

func (s *crossmodelMockSuite) TestAddRelationFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "AddRelation")

			return errors.New(msg)
		})
	client := crossmodel.NewClient(apiCaller)
	results, err := client.AddRelation("")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}
