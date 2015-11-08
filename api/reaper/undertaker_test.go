// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reaper_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/reaper"
	"github.com/juju/juju/apiserver/params"
)

var _ reaper.ReaperClient = (*reaper.Client)(nil)

func (s *reaperSuite) TestEnvironInfo(c *gc.C) {
	var called bool
	client := s.mockClient(c, "EnvironInfo", func(response interface{}) {
		called = true
		result := response.(*params.ReaperEnvironInfoResult)
		result.Result = params.ReaperEnvironInfo{}
	})

	result, err := client.EnvironInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(result, gc.Equals, params.ReaperEnvironInfoResult{})
}

func (s *reaperSuite) TestProcessDyingEnviron(c *gc.C) {
	var called bool
	client := s.mockClient(c, "ProcessDyingEnviron", func(response interface{}) {
		called = true
		c.Assert(response, gc.IsNil)
	})

	c.Assert(client.ProcessDyingEnviron(), jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *reaperSuite) TestRemoveEnviron(c *gc.C) {
	var called bool
	client := s.mockClient(c, "RemoveEnviron", func(response interface{}) {
		called = true
		c.Assert(response, gc.IsNil)
	})

	err := client.RemoveEnviron()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *reaperSuite) mockClient(c *gc.C, expectedRequest string, callback func(response interface{})) *reaper.Client {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			args, response interface{},
		) error {
			c.Check(objType, gc.Equals, "Reaper")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, expectedRequest)

			a, ok := args.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(a.Entities, gc.DeepEquals, []params.Entity{{Tag: "environment-"}})

			callback(response)
			return nil
		})

	return reaper.NewClient(apiCaller)
}
