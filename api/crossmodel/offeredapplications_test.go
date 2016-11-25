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

type offeredApplicationsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&offeredApplicationsSuite{})

func offeredApplicationsCaller(c *gc.C, offers []params.OfferedApplication, err string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "OfferedApplications")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "OfferedApplications")

			args, ok := a.(params.ApplicationURLs)
			c.Assert(ok, jc.IsTrue)

			url := args.ApplicationURLs[0]
			c.Check(url, gc.DeepEquals, "local:/u/user/servicename")

			offersByURL := make(map[string]params.OfferedApplication)
			for _, offer := range offers {
				offersByURL[offer.ApplicationURL] = offer
			}
			if results, ok := result.(*params.OfferedApplicationResults); ok {
				results.Results = make([]params.OfferedApplicationResult, len(args.ApplicationURLs))
				for i, url := range args.ApplicationURLs {
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

func (s *offeredApplicationsSuite) TestOfferedApplications(c *gc.C) {
	offers := []params.OfferedApplication{
		{
			ApplicationURL:  "local:/u/user/servicename",
			ApplicationName: "service",
			CharmName:       "charm",
			Description:     "description",
			Endpoints:       map[string]string{"foo": "bar"},
			Registered:      true,
		},
	}
	apiCaller := offeredApplicationsCaller(c, offers, "")
	client := crossmodel.NewOfferedApplications(apiCaller)
	result, offerErrors, err := client.OfferedApplications("local:/u/user/servicename")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(offerErrors, gc.HasLen, 0)
	expectedOffer := crossmodel.MakeOfferedApplicationFromParams(offers[0])
	c.Assert(result["local:/u/user/servicename"], jc.DeepEquals, expectedOffer)
}

func (s *offeredApplicationsSuite) TestOfferedApplicationsNotFound(c *gc.C) {
	apiCaller := offeredApplicationsCaller(c, nil, "")
	client := crossmodel.NewOfferedApplications(apiCaller)
	result, offerErrors, err := client.OfferedApplications("local:/u/user/servicename")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)
	c.Assert(offerErrors, gc.HasLen, 1)
	c.Assert(offerErrors["local:/u/user/servicename"], jc.Satisfies, errors.IsNotFound)
}

func (s *offeredApplicationsSuite) TestOfferedApplicationsError(c *gc.C) {
	apiCaller := offeredApplicationsCaller(c, nil, "error")
	client := crossmodel.NewOfferedApplications(apiCaller)
	_, offerErrors, err := client.OfferedApplications("local:/u/user/servicename")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offerErrors, gc.HasLen, 1)
	c.Assert(offerErrors["local:/u/user/servicename"], gc.ErrorMatches, `error retrieving offer at "local:/u/user/servicename": error`)
}

func watchOfferedApplicationsCaller(c *gc.C, err string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "OfferedApplications")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "WatchOfferedApplications")
			c.Assert(a, gc.IsNil)

			if result, ok := result.(*params.StringsWatchResult); ok {
				result.Error = &params.Error{Message: "fail"}
			}
			return nil
		})
}

func (s *offeredApplicationsSuite) TestWatchOfferedApplications(c *gc.C) {
	apiCaller := watchOfferedApplicationsCaller(c, "")
	client := crossmodel.NewOfferedApplications(apiCaller)
	_, err := client.WatchOfferedApplications()
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *offeredApplicationsSuite) TestWatchOfferedApplicationsFacadeCallError(c *gc.C) {
	client := crossmodel.NewOfferedApplications(apiCallerWithError(c, "OfferedApplications", "WatchOfferedApplications"))
	_, err := client.WatchOfferedApplications()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "facade failure")
}

func (s *offeredApplicationsSuite) TestOfferedApplicationsFacadeCallError(c *gc.C) {
	client := crossmodel.NewOfferedApplications(apiCallerWithError(c, "OfferedApplications", "OfferedApplications"))
	_, _, err := client.OfferedApplications("url")
	c.Assert(errors.Cause(err), gc.ErrorMatches, "facade failure")
}
