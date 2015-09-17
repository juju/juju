// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/undertaker"
	"github.com/juju/juju/apiserver/params"
)

var _ undertaker.UndertakerClient = (*undertaker.Client)(nil)

func (s *undertakerSuite) TestEnvironInfo(c *gc.C) {
	var called bool
	client := s.mockClient(c, "EnvironInfo", func(response interface{}) {
		called = true
		result := response.(*params.UndertakerEnvironInfoResult)
		result.Result = params.UndertakerEnvironInfo{}
	})

	result, err := client.EnvironInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(result, gc.Equals, params.UndertakerEnvironInfoResult{})
}

func (s *undertakerSuite) TestProcessDyingEnviron(c *gc.C) {
	var called bool
	client := s.mockClient(c, "ProcessDyingEnviron", func(response interface{}) {
		called = true
		c.Assert(response, gc.IsNil)
	})

	c.Assert(client.ProcessDyingEnviron(), jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *undertakerSuite) TestRemoveEnviron(c *gc.C) {
	var called bool
	client := s.mockClient(c, "RemoveEnviron", func(response interface{}) {
		called = true
		c.Assert(response, gc.IsNil)
	})

	err := client.RemoveEnviron()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *undertakerSuite) mockClient(c *gc.C, expectedRequest string, callback func(response interface{})) *undertaker.Client {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			args, response interface{},
		) error {
			c.Check(objType, gc.Equals, "Undertaker")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, expectedRequest)

			a, ok := args.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(a.Entities, gc.DeepEquals, []params.Entity{{Tag: "environment-"}})

			callback(response)
			return nil
		})

	return undertaker.NewClient(apiCaller)
}
