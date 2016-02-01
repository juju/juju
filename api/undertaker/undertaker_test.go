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
	client := s.mockClient(c, "ModelInfo", func(response interface{}) {
		called = true
		result := response.(*params.UndertakerModelInfoResult)
		result.Result = params.UndertakerModelInfo{}
	})

	result, err := client.ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(result, gc.Equals, params.UndertakerModelInfoResult{})
}

func (s *undertakerSuite) TestProcessDyingEnviron(c *gc.C) {
	var called bool
	client := s.mockClient(c, "ProcessDyingModel", func(response interface{}) {
		called = true
		c.Assert(response, gc.IsNil)
	})

	c.Assert(client.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *undertakerSuite) TestRemoveModel(c *gc.C) {
	var called bool
	client := s.mockClient(c, "RemoveModel", func(response interface{}) {
		called = true
		c.Assert(response, gc.IsNil)
	})

	err := client.RemoveModel()
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
			c.Check(a.Entities, gc.DeepEquals, []params.Entity{{Tag: coretesting.ModelTag.String()}})

			callback(response)
			return nil
		})

	return undertaker.NewClient(apiCaller)
}

func (s *undertakerSuite) TestWatchModelResourcesGetsChange(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			args, response interface{},
		) error {
			if resp, ok := response.(*params.NotifyWatchResults); ok {
				c.Check(objType, gc.Equals, "Undertaker")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "WatchModelResources")

				a, ok := args.(params.Entities)
				c.Check(ok, jc.IsTrue)
				c.Check(a.Entities, gc.DeepEquals, []params.Entity{{Tag: coretesting.ModelTag.String()}})

				resp.Results = []params.NotifyWatchResult{{NotifyWatcherId: "1"}}
			} else {
				c.Check(objType, gc.Equals, "NotifyWatcher")
				c.Check(id, gc.Equals, "1")
				c.Check(request, gc.Equals, "Next")
			}
			return nil
		})

	client := undertaker.NewClient(apiCaller)
	w, err := client.WatchModelResources()
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-w.Changes():
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for change")
	}
}

func (s *undertakerSuite) TestWatchModelResourcesError(c *gc.C) {
	var called bool

	// The undertaker feature tests ensure WatchModelResources is connected
	// correctly end to end. This test just ensures that the API calls work.
	client := s.mockClient(c, "WatchModelResources", func(response interface{}) {
		called = true
		c.Check(response, gc.DeepEquals, &params.NotifyWatchResults{Results: []params.NotifyWatchResult(nil)})
	})

	w, err := client.WatchModelResources()
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
	c.Assert(w, gc.IsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *undertakerSuite) TestModelConfig(c *gc.C) {
	var called bool

	// The undertaker feature tests ensure ModelConfig is connected
	// correctly end to end. This test just ensures that the API calls work.
	client := s.mockClient(c, "ModelConfig", func(response interface{}) {
		called = true
		c.Check(response, gc.DeepEquals, &params.ModelConfigResult{Config: params.ModelConfig(nil)})
	})

	// We intentionally don't test the error here. We are only interested that
	// the ModelConfig endpoint was called.
	client.ModelConfig()
	c.Assert(called, jc.IsTrue)
}
