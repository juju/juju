// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/storage"
)

type serviceSuite struct {
	jujutesting.JujuConnSuite

	client *application.Client
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.client = application.NewClient(s.APIState)
	c.Assert(s.client, gc.NotNil)
}

func (s *serviceSuite) TestSetServiceMetricCredentials(c *gc.C) {
	var called bool
	application.PatchFacadeCall(s, s.client, func(request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "SetMetricCredentials")
		args, ok := a.(params.ApplicationMetricCredentials)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.Creds, gc.HasLen, 1)
		c.Assert(args.Creds[0].ApplicationName, gc.Equals, "serviceA")
		c.Assert(args.Creds[0].MetricCredentials, gc.DeepEquals, []byte("creds 1"))

		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})
	err := s.client.SetMetricCredentials("serviceA", []byte("creds 1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *serviceSuite) TestSetServiceMetricCredentialsFails(c *gc.C) {
	var called bool
	application.PatchFacadeCall(s, s.client, func(request string, args, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "SetMetricCredentials")
		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		result.Results[0].Error = common.ServerError(common.ErrPerm)
		return result.OneError()
	})
	err := s.client.SetMetricCredentials("application", []byte("creds"))
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(called, jc.IsTrue)
}

func (s *serviceSuite) TestSetServiceMetricCredentialsNoMocks(c *gc.C) {
	application := s.Factory.MakeApplication(c, nil)
	err := s.client.SetMetricCredentials(application.Name(), []byte("creds"))
	c.Assert(err, jc.ErrorIsNil)
	err = application.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(application.MetricCredentials(), gc.DeepEquals, []byte("creds"))
}

func (s *serviceSuite) TestSetServiceDeploy(c *gc.C) {
	var called bool
	application.PatchFacadeCall(s, s.client, func(request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "Deploy")
		args, ok := a.(params.ApplicationsDeploy)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.Applications, gc.HasLen, 1)
		c.Assert(args.Applications[0].CharmUrl, gc.Equals, "cs:trusty/a-charm-1")
		c.Assert(args.Applications[0].ApplicationName, gc.Equals, "serviceA")
		c.Assert(args.Applications[0].Series, gc.Equals, "series")
		c.Assert(args.Applications[0].NumUnits, gc.Equals, 2)
		c.Assert(args.Applications[0].ConfigYAML, gc.Equals, "configYAML")
		c.Assert(args.Applications[0].Constraints, gc.DeepEquals, constraints.MustParse("mem=4G"))
		c.Assert(args.Applications[0].Placement, gc.DeepEquals, []*instance.Placement{{"scope", "directive"}})
		c.Assert(args.Applications[0].EndpointBindings, gc.DeepEquals, map[string]string{"foo": "bar"})
		c.Assert(args.Applications[0].Storage, gc.DeepEquals, map[string]storage.Constraints{"data": storage.Constraints{Pool: "pool"}})
		c.Assert(args.Applications[0].Resources, gc.DeepEquals, map[string]string{"foo": "bar"})

		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})

	args := application.DeployArgs{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("trusty/a-charm-1"),
		},
		ApplicationName:  "serviceA",
		Series:           "series",
		NumUnits:         2,
		ConfigYAML:       "configYAML",
		Cons:             constraints.MustParse("mem=4G"),
		Placement:        []*instance.Placement{{"scope", "directive"}},
		Storage:          map[string]storage.Constraints{"data": storage.Constraints{Pool: "pool"}},
		Resources:        map[string]string{"foo": "bar"},
		EndpointBindings: map[string]string{"foo": "bar"},
	}
	err := s.client.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *serviceSuite) TestServiceGetCharmURL(c *gc.C) {
	var called bool
	application.PatchFacadeCall(s, s.client, func(request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "GetCharmURL")
		args, ok := a.(params.ApplicationGet)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.ApplicationName, gc.Equals, "application")

		result := response.(*params.StringResult)
		result.Result = "curl"
		return nil
	})
	curl, err := s.client.GetCharmURL("application")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, charm.MustParseURL("curl"))
	c.Assert(called, jc.IsTrue)
}

func (s *serviceSuite) TestServiceSetCharm(c *gc.C) {
	var called bool
	application.PatchFacadeCall(s, s.client, func(request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "SetCharm")
		args, ok := a.(params.ApplicationSetCharm)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.ApplicationName, gc.Equals, "application")
		c.Assert(args.CharmUrl, gc.Equals, "cs:trusty/application-1")
		c.Assert(args.ForceSeries, gc.Equals, true)
		c.Assert(args.ForceUnits, gc.Equals, true)
		return nil
	})
	cfg := application.SetCharmConfig{
		ApplicationName: "application",
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("trusty/application-1"),
		},
		ForceSeries: true,
		ForceUnits:  true,
	}
	err := s.client.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
