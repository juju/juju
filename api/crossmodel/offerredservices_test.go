// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type offeredServicesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&offeredServicesSuite{})

func offeredServicesCaller(c *gc.C, offers []params.OfferedService, err string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "OfferedServices")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "OfferedServices")

			args, ok := a.(params.OfferedServiceQueryParams)
			c.Assert(ok, jc.IsTrue)

			url := args.ServiceUrls[0]
			c.Check(url, gc.DeepEquals, "local:/u/user/servicename")

			offersByURL := make(map[string]params.OfferedService)
			for _, offer := range offers {
				offersByURL[offer.ServiceURL] = offer
			}
			if results, ok := result.(*params.OfferedServiceResults); ok {
				results.Results = make([]params.OfferedServiceResult, len(args.ServiceUrls))
				for i, url := range args.ServiceUrls {
					if err != "" {
						results.Results[i].Error = common.ServerError(errors.New(err))
						continue
					}
					if offer, ok := offersByURL[url]; ok {
						results.Results[i].Result = offer
					} else {
						results.Results[i].Error = common.ServerError(errors.NotFoundf("offfer at url %q", url))
					}
				}
			}
			return nil
		})
}

func (s *offeredServicesSuite) TestOfferedServices(c *gc.C) {
	offers := []params.OfferedService{
		{
			ServiceURL:  "local:/u/user/servicename",
			ServiceName: "service",
			CharmName:   "charm",
			Description: "description",
			Endpoints:   map[string]string{"foo": "bar"},
			Registered:  true,
		},
	}
	apiCaller := offeredServicesCaller(c, offers, "")
	client := crossmodel.NewOfferedServices(apiCaller)
	result, offerErrors, err := client.OfferedServices("local:/u/user/servicename")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(offerErrors, gc.HasLen, 0)
	expectedOffer := crossmodel.MakeOfferedServiceFromParams(offers[0])
	c.Assert(result["local:/u/user/servicename"], jc.DeepEquals, expectedOffer)
}

func (s *offeredServicesSuite) TestOfferedServicesNotFound(c *gc.C) {
	apiCaller := offeredServicesCaller(c, nil, "")
	client := crossmodel.NewOfferedServices(apiCaller)
	result, offerErrors, err := client.OfferedServices("local:/u/user/servicename")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)
	c.Assert(offerErrors, gc.HasLen, 1)
	c.Assert(offerErrors["local:/u/user/servicename"], jc.Satisfies, errors.IsNotFound)
}

func (s *offeredServicesSuite) TestOfferedServicesError(c *gc.C) {
	apiCaller := offeredServicesCaller(c, nil, "error")
	client := crossmodel.NewOfferedServices(apiCaller)
	_, offerErrors, err := client.OfferedServices("local:/u/user/servicename")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offerErrors, gc.HasLen, 1)
	c.Assert(offerErrors["local:/u/user/servicename"], gc.ErrorMatches, `error retrieving offer at "local:/u/user/servicename": error`)
}

func watchOfferedServicesCaller(c *gc.C, err string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "OfferedServices")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "WatchOfferedServices")
			c.Assert(a, gc.IsNil)

			if result, ok := result.(*params.StringsWatchResult); ok {
				result.Error = &params.Error{Message: "fail"}
			}
			return nil
		})
}

func (s *offeredServicesSuite) TestWatchOfferedServices(c *gc.C) {
	apiCaller := watchOfferedServicesCaller(c, "")
	client := crossmodel.NewOfferedServices(apiCaller)
	_, err := client.WatchOfferedServices()
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *offeredServicesSuite) TestWatchOfferedServicesFacadeCallError(c *gc.C) {
	client := crossmodel.NewOfferedServices(apiCallerWithError(c, "OfferedServices", "WatchOfferedServices"))
	_, err := client.WatchOfferedServices()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "facade failure")
}

func (s *offeredServicesSuite) TestOfferedServicesFacadeCallError(c *gc.C) {
	client := crossmodel.NewOfferedServices(apiCallerWithError(c, "OfferedServices", "OfferedServices"))
	_, _, err := client.OfferedServices("url")
	c.Assert(errors.Cause(err), gc.ErrorMatches, "facade failure")
}
