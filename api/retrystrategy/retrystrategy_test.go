// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	"fmt"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/retrystrategy"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type retryStrategySuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&retryStrategySuite{})

func (s *retryStrategySuite) TestRetryStrategyOk(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	expectedRetryStrategy := params.RetryStrategy{
		ShouldRetry: true,
	}
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "RetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.RetryStrategyResults{})
		result := response.(*params.RetryStrategyResults)
		result.Results = []params.RetryStrategyResult{{
			Result: &expectedRetryStrategy,
		}}
		return nil
	})

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	retryStrategy, err := client.RetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(retryStrategy, jc.DeepEquals, expectedRetryStrategy)
}

func (s *retryStrategySuite) TestRetryStrategyResultError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "RetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.RetryStrategyResults{})
		result := response.(*params.RetryStrategyResults)
		result.Results = []params.RetryStrategyResult{{
			Error: &params.Error{
				Message: "splat",
				Code:    params.CodeNotAssigned,
			},
		}}
		return nil
	})

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	retryStrategy, err := client.RetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(retryStrategy, jc.DeepEquals, params.RetryStrategy{})
}

func (s *retryStrategySuite) TestRetryStrategyMoreResults(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "RetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.RetryStrategyResults{})
		result := response.(*params.RetryStrategyResults)
		result.Results = make([]params.RetryStrategyResult, 2)
		return nil
	})

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	retryStrategy, err := client.RetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(retryStrategy, jc.DeepEquals, params.RetryStrategy{})
}

func (s *retryStrategySuite) TestRetryStrategyError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "RetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.RetryStrategyResults{})
		return fmt.Errorf("impossibru")
	})

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	retryStrategy, err := client.RetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "impossibru")
	c.Assert(retryStrategy, jc.DeepEquals, params.RetryStrategy{})
}

func (s *retryStrategySuite) TestWatchRetryStrategyError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "RetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchRetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = make([]params.NotifyWatchResult, 1)
		return fmt.Errorf("sosorry")
	})

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	w, err := client.WatchRetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "sosorry")
	c.Assert(w, gc.IsNil)
}

func (s *retryStrategySuite) TestWatchRetryStrategyResultError(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "RetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchRetryStrategy")
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

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	w, err := client.WatchRetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "rigged")
	c.Assert(w, gc.IsNil)
}

func (s *retryStrategySuite) TestWatchRetryStrategyMoreResults(c *gc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, gc.Equals, "RetryStrategy")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchRetryStrategy")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, gc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = make([]params.NotifyWatchResult, 2)
		return nil
	})

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)

	w, err := client.WatchRetryStrategy(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(w, gc.IsNil)
}
