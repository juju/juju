// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/service"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
)

type serviceSuite struct {
	jujutesting.JujuConnSuite

	client *service.Client
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.client = service.NewClient(s.APIState)
	c.Assert(s.client, gc.NotNil)
}

func (s *serviceSuite) TestSetServiceMetricCredentials(c *gc.C) {
	var called bool
	service.PatchFacadeCall(s, s.client, func(request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "SetServiceMetricCredentials")
		args, ok := a.(params.ServiceMetricCredentials)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.Creds, gc.HasLen, 2)
		arguments := map[string][]byte{}
		for _, arg := range args.Creds {
			_, ok := arguments[arg.ServiceName]
			c.Assert(ok, jc.IsFalse)
			arguments[arg.ServiceName] = arg.MetricCredentials
		}
		c.Assert(arguments["serviceA"], gc.DeepEquals, []byte("creds 1"))
		c.Assert(arguments["serviceB"], gc.DeepEquals, []byte("creds 2"))

		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})
	args := map[string][]byte{
		"serviceA": []byte("creds 1"),
		"serviceB": []byte("creds 2"),
	}
	err := s.client.SetServiceMetricCredentials(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *serviceSuite) TestSetServiceMetricCredentialsFails(c *gc.C) {
	var called bool
	service.PatchFacadeCall(s, s.client, func(request string, args, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "SetServiceMetricCredentials")
		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		result.Results[0].Error = common.ServerError(common.ErrPerm)
		return result.OneError()
	})
	err := s.client.SetServiceMetricCredentials(map[string][]byte{"service": []byte("creds")})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(called, jc.IsTrue)
}
