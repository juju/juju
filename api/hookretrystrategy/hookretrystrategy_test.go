// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package hookretrystrategy_test

import (
	"fmt"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/hookretrystrategy"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type hookRetryStrategySuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&hookRetryStrategySuite{})

func (s *hookRetryStrategySuite) TestHookRetryStrategyOk(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	expectedRetryStrategy := params.HookRetryStrategy{
		ShouldRetry: true,
	}
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "HookRetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "HookRetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.HookRetryStrategyResults{})
		result := response.(*params.HookRetryStrategyResults)
		result.Results = []params.HookRetryStrategyResult{{
			Result: &expectedRetryStrategy,
		}}
		return nil
	})

	client := hookretrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	retryStrategy, err := client.HookRetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(retryStrategy, jc.DeepEquals, expectedRetryStrategy)
}

func (s *hookRetryStrategySuite) TestHookRetryStrategyResultError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "HookRetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "HookRetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.HookRetryStrategyResults{})
		result := response.(*params.HookRetryStrategyResults)
		result.Results = []params.HookRetryStrategyResult{{
			Error: &params.Error{
				Message: "splat",
				Code:    params.CodeNotAssigned,
			},
		}}
		return nil
	})

	client := hookretrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	retryStrategy, err := client.HookRetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(retryStrategy, jc.DeepEquals, params.HookRetryStrategy{})
}

func (s *hookRetryStrategySuite) TestHookRetryStrategyMoreResults(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "HookRetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "HookRetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.HookRetryStrategyResults{})
		result := response.(*params.HookRetryStrategyResults)
		result.Results = make([]params.HookRetryStrategyResult, 2)
		return nil
	})

	client := hookretrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	retryStrategy, err := client.HookRetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(retryStrategy, jc.DeepEquals, params.HookRetryStrategy{})
}

func (s *hookRetryStrategySuite) TestHookRetryStrategyError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "HookRetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "HookRetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.HookRetryStrategyResults{})
		return fmt.Errorf("impossibru")
	})

	client := hookretrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	retryStrategy, err := client.HookRetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "impossibru")
	c.Assert(retryStrategy, jc.DeepEquals, params.HookRetryStrategy{})
}

func (s *hookRetryStrategySuite) TestWatchHookRetryStrategyError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "HookRetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchHookRetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = make([]params.NotifyWatchResult, 1)
		return fmt.Errorf("sosorry")
	})

	client := hookretrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	w, err := client.WatchHookRetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "sosorry")
	c.Assert(w, gc.IsNil)
}

func (s *hookRetryStrategySuite) TestWatchHookRetryStrategyResultError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "HookRetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchHookRetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = []params.NotifyWatchResult{{
			Error: &params.Error{
				Message: "rigged",
				Code:    params.CodeNotAssigned,
			},
		}}
		return nil
	})

	client := hookretrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	w, err := client.WatchHookRetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "rigged")
	c.Assert(w, gc.IsNil)
}

func (s *hookRetryStrategySuite) TestWatchHookRetryStrategyMoreResults(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "HookRetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchHookRetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = make([]params.NotifyWatchResult, 2)
		return nil
	})

	client := hookretrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	w, err := client.WatchHookRetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(w, gc.IsNil)
}
