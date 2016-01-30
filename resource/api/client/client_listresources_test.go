// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/client"
)

var _ = gc.Suite(&ListResourcesSuite{})

type ListResourcesSuite struct {
	BaseSuite
}

func (s *ListResourcesSuite) TestOkay(c *gc.C) {
	expected, apiResult := newResourceResult(c, "a-service", "spam")
	s.facade.apiResults["a-service"] = apiResult

	cl := client.NewClient(s.facade, s, s.facade)

	services := []string{"a-service"}
	results, err := cl.ListResources(services)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, []resource.ServiceResources{
		{Resources: expected},
	})
	c.Check(s.stub.Calls(), gc.HasLen, 1)
	s.stub.CheckCall(c, 0, "FacadeCall",
		"ListResources",
		&api.ListResourcesArgs{[]params.Entity{{
			Tag: "service-a-service",
		}}},
		&api.ResourcesResults{
			Results: []api.ResourcesResult{
				apiResult,
			},
		},
	)
}

func (s *ListResourcesSuite) TestBulk(c *gc.C) {
	expected1, apiResult1 := newResourceResult(c, "a-service", "spam")
	s.facade.apiResults["a-service"] = apiResult1
	expected2, apiResult2 := newResourceResult(c, "other-service", "eggs", "ham")
	s.facade.apiResults["other-service"] = apiResult2

	cl := client.NewClient(s.facade, s, s.facade)

	services := []string{"a-service", "other-service"}
	results, err := cl.ListResources(services)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, []resource.ServiceResources{
		{Resources: expected1},
		{Resources: expected2},
	})
	c.Check(s.stub.Calls(), gc.HasLen, 1)
	s.stub.CheckCall(c, 0, "FacadeCall",
		"ListResources",
		&api.ListResourcesArgs{[]params.Entity{
			{
				Tag: "service-a-service",
			}, {
				Tag: "service-other-service",
			},
		}},
		&api.ResourcesResults{
			Results: []api.ResourcesResult{
				apiResult1,
				apiResult2,
			},
		},
	)
}

func (s *ListResourcesSuite) TestNoServices(c *gc.C) {
	cl := client.NewClient(s.facade, s, s.facade)

	var services []string
	results, err := cl.ListResources(services)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "FacadeCall")
}

func (s *ListResourcesSuite) TestBadServices(c *gc.C) {
	cl := client.NewClient(s.facade, s, s.facade)

	services := []string{"???"}
	_, err := cl.ListResources(services)

	c.Check(err, gc.ErrorMatches, `.*invalid service.*`)
	s.stub.CheckNoCalls(c)
}

func (s *ListResourcesSuite) TestServiceNotFound(c *gc.C) {
	cl := client.NewClient(s.facade, s, s.facade)

	services := []string{"a-service"}
	_, err := cl.ListResources(services)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.stub.CheckCallNames(c, "FacadeCall")
}

func (s *ListResourcesSuite) TestServiceEmpty(c *gc.C) {
	s.facade.apiResults["a-service"] = api.ResourcesResult{}

	cl := client.NewClient(s.facade, s, s.facade)

	services := []string{"a-service"}
	results, err := cl.ListResources(services)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, []resource.ServiceResources{
		{},
	})
	s.stub.CheckCallNames(c, "FacadeCall")
}

func (s *ListResourcesSuite) TestServerError(c *gc.C) {
	failure := errors.New("<failure>")
	s.facade.FacadeCallFn = func(_ string, _, _ interface{}) error {
		return failure
	}

	cl := client.NewClient(s.facade, s, s.facade)

	services := []string{"a-service"}
	_, err := cl.ListResources(services)

	c.Check(err, gc.ErrorMatches, `<failure>`)
	s.stub.CheckCallNames(c, "FacadeCall")
}

func (s *ListResourcesSuite) TestTooFew(c *gc.C) {
	s.facade.FacadeCallFn = func(_ string, _, response interface{}) error {
		typedResponse, ok := response.(*api.ResourcesResults)
		c.Assert(ok, jc.IsTrue)

		typedResponse.Results = []api.ResourcesResult{{
			Resources: nil,
		}}

		return nil
	}

	cl := client.NewClient(s.facade, s, s.facade)

	services := []string{"a-service", "other-service"}
	results, err := cl.ListResources(services)

	c.Check(results, gc.HasLen, 0)
	c.Check(err, gc.ErrorMatches, `.*got invalid data from server \(expected 2 results, got 1\).*`)
	s.stub.CheckCallNames(c, "FacadeCall")
}

func (s *ListResourcesSuite) TestTooMany(c *gc.C) {
	s.facade.FacadeCallFn = func(_ string, _, response interface{}) error {
		typedResponse, ok := response.(*api.ResourcesResults)
		c.Assert(ok, jc.IsTrue)

		typedResponse.Results = []api.ResourcesResult{{
			Resources: nil,
		}, {
			Resources: nil,
		}, {
			Resources: nil,
		}}

		return nil
	}

	cl := client.NewClient(s.facade, s, s.facade)

	services := []string{"a-service", "other-service"}
	results, err := cl.ListResources(services)

	c.Check(results, gc.HasLen, 0)
	c.Check(err, gc.ErrorMatches, `.*got invalid data from server \(expected 2 results, got 3\).*`)
	s.stub.CheckCallNames(c, "FacadeCall")
}

func (s *ListResourcesSuite) TestConversionFailed(c *gc.C) {
	s.facade.FacadeCallFn = func(_ string, _, response interface{}) error {
		typedResponse, ok := response.(*api.ResourcesResults)
		c.Assert(ok, jc.IsTrue)

		var res api.Resource
		res.Name = "spam"
		typedResponse.Results = []api.ResourcesResult{{
			Resources: []api.Resource{
				res,
			},
		}}

		return nil
	}

	cl := client.NewClient(s.facade, s, s.facade)

	services := []string{"a-service"}
	_, err := cl.ListResources(services)

	c.Check(err, gc.ErrorMatches, `.*got bad data.*`)
	s.stub.CheckCallNames(c, "FacadeCall")
}
