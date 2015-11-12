// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
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

			args, ok := a.(params.CrossModelOffers)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Offers, gc.HasLen, 1)

			offer := args.Offers[0]
			c.Assert(offer.Service, gc.DeepEquals, service)
			c.Assert(offer.Endpoints, jc.SameContents, []string{endPointA, endPointB})
			c.Assert(offer.URL, gc.DeepEquals, url)
			c.Assert(offer.Users, jc.SameContents, []string{user1, user2})

			if results, k := result.(*params.ErrorResults); k {
				all := make([]params.ErrorResult, len(args.Offers))
				// add one error to make sure it's catered for.
				all = append(all, params.ErrorResult{
					Error: common.ServerError(errors.New(msg))})
				results.Results = all
			}

			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	results, err := client.Offer(service, []string{endPointA, endPointB}, url, []string{user1, user2})
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
	results, err := client.Offer("", nil, "", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}

func (s *crossmodelMockSuite) TestShow(c *gc.C) {
	url := "local:/u/fred/prod/db2"

	desc := "IBM DB2 Express Server Edition is an entry level database system"
	endpoints := []params.RemoteEndpoint{
		params.RemoteEndpoint{"db2", "mysql", "provider"},
		params.RemoteEndpoint{"log", "http", "requirer"},
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

			args, ok := a.(params.EndpointsSearchFilter)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.URL, gc.DeepEquals, url)

			if points, k := result.(*params.RemoteServiceInfos); k {
				points.Result = []params.RemoteServiceInfo{params.RemoteServiceInfo{
					Description: desc,
					Endpoints:   endpoints,
					Service:     serviceTag,
				}}
			}
			return nil
		})
	client := crossmodel.NewClient(apiCaller)
	found, err := client.Show(url)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(called, jc.IsTrue)
	c.Assert(found, gc.DeepEquals, params.RemoteServiceInfo{
		Description: desc,
		Endpoints:   endpoints,
		Service:     serviceTag})
}

func (s *crossmodelMockSuite) TestShowMultiple(c *gc.C) {
	url := "local:/u/fred/prod/db2"

	desc := "IBM DB2 Express Server Edition is an entry level database system"
	endpoints := []params.RemoteEndpoint{
		params.RemoteEndpoint{"db2", "mysql", "provider"},
		params.RemoteEndpoint{"log", "http", "requirer"},
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

			args, ok := a.(params.EndpointsSearchFilter)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.URL, gc.DeepEquals, url)

			if points, k := result.(*params.RemoteServiceInfos); k {
				points.Result = []params.RemoteServiceInfo{
					params.RemoteServiceInfo{
						Description: desc,
						Endpoints:   endpoints,
						Service:     serviceTag,
					},
					params.RemoteServiceInfo{
						Description: desc,
						Endpoints:   endpoints,
						Service:     serviceTag,
					}}
			}
			return nil
		})
	client := crossmodel.NewClient(apiCaller)
	found, err := client.Show(url)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`expected to find one result for url %q but found 2`, url))
	c.Assert(found, gc.DeepEquals, params.RemoteServiceInfo{})

	c.Assert(called, jc.IsTrue)
}

func (s *crossmodelMockSuite) TestShowFacadeCallError(c *gc.C) {
	url := "local:/u/fred/prod/db2"
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
	c.Assert(found, gc.DeepEquals, params.RemoteServiceInfo{})
}
