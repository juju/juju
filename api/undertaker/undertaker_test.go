// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/undertaker"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
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
			c.Check(ok, jc.IsTrue)
			c.Check(a.Entities, gc.DeepEquals, []params.Entity{{Tag: "environment-"}})

			callback(response)
			return nil
		})

	return undertaker.NewClient(apiCaller)
}

func (s *undertakerSuite) TestWatchEnvironResourcesGetsChange(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			args, response interface{},
		) error {
			if resp, ok := response.(*params.NotifyWatchResults); ok {
				c.Check(objType, gc.Equals, "Undertaker")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "WatchEnvironResources")

				a, ok := args.(params.Entities)
				c.Check(ok, jc.IsTrue)
				c.Check(a.Entities, gc.DeepEquals, []params.Entity{{Tag: "environment-"}})

				resp.Results = []params.NotifyWatchResult{{NotifyWatcherId: "1"}}
			} else {
				c.Check(objType, gc.Equals, "NotifyWatcher")
				c.Check(id, gc.Equals, "1")
				c.Check(request, gc.Equals, "Next")
			}
			return nil
		})

	client := undertaker.NewClient(apiCaller)
	w, err := client.WatchEnvironResources()
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-w.Changes():
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for change")
	}
}

func (s *undertakerSuite) TestWatchEnvironResourcesError(c *gc.C) {
	var called bool

	// The undertaker feature tests ensure WatchEnvironResources is connected
	// correctly end to end. This test just ensures that the API calls work.
	client := s.mockClient(c, "WatchEnvironResources", func(response interface{}) {
		called = true
		c.Check(response, gc.DeepEquals, &params.NotifyWatchResults{Results: []params.NotifyWatchResult(nil)})
	})

	w, err := client.WatchEnvironResources()
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
	c.Assert(w, gc.IsNil)
	c.Assert(called, jc.IsTrue)
}
