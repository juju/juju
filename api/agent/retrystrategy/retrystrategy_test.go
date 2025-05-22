// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/retrystrategy"
	"github.com/juju/juju/api/base/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type retryStrategySuite struct {
	coretesting.BaseSuite
}

func TestRetryStrategySuite(t *stdtesting.T) {
	tc.Run(t, &retryStrategySuite{})
}

func (s *retryStrategySuite) TestRetryStrategyOk(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	expectedRetryStrategy := params.RetryStrategy{
		ShouldRetry: true,
	}
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, tc.Equals, "RetryStrategy")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RetryStrategy")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.RetryStrategyResults{})
		result := response.(*params.RetryStrategyResults)
		result.Results = []params.RetryStrategyResult{{
			Result: &expectedRetryStrategy,
		}}
		return nil
	})

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)

	retryStrategy, err := client.RetryStrategy(c.Context(), tag)
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(retryStrategy, tc.DeepEquals, expectedRetryStrategy)
}

func (s *retryStrategySuite) TestRetryStrategyResultError(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, tc.Equals, "RetryStrategy")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RetryStrategy")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.RetryStrategyResults{})
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
	c.Assert(client, tc.NotNil)

	retryStrategy, err := client.RetryStrategy(c.Context(), tag)
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "splat")
	c.Assert(retryStrategy, tc.DeepEquals, params.RetryStrategy{})
}

func (s *retryStrategySuite) TestRetryStrategyMoreResults(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, tc.Equals, "RetryStrategy")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RetryStrategy")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.RetryStrategyResults{})
		result := response.(*params.RetryStrategyResults)
		result.Results = make([]params.RetryStrategyResult, 2)
		return nil
	})

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)

	retryStrategy, err := client.RetryStrategy(c.Context(), tag)
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(retryStrategy, tc.DeepEquals, params.RetryStrategy{})
}

func (s *retryStrategySuite) TestRetryStrategyError(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, tc.Equals, "RetryStrategy")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RetryStrategy")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.RetryStrategyResults{})
		return fmt.Errorf("impossibru")
	})

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)

	retryStrategy, err := client.RetryStrategy(c.Context(), tag)
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "impossibru")
	c.Assert(retryStrategy, tc.DeepEquals, params.RetryStrategy{})
}

func (s *retryStrategySuite) TestWatchRetryStrategyError(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, tc.Equals, "RetryStrategy")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchRetryStrategy")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = make([]params.NotifyWatchResult, 1)
		return fmt.Errorf("sosorry")
	})

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)

	w, err := client.WatchRetryStrategy(c.Context(), tag)
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "sosorry")
	c.Assert(w, tc.IsNil)
}

func (s *retryStrategySuite) TestWatchRetryStrategyResultError(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, tc.Equals, "RetryStrategy")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchRetryStrategy")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.NotifyWatchResults{})
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
	c.Assert(client, tc.NotNil)

	w, err := client.WatchRetryStrategy(c.Context(), tag)
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "rigged")
	c.Assert(w, tc.IsNil)
}

func (s *retryStrategySuite) TestWatchRetryStrategyMoreResults(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		called = true

		c.Check(objType, tc.Equals, "RetryStrategy")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchRetryStrategy")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = make([]params.NotifyWatchResult, 2)
		return nil
	})

	client := retrystrategy.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)

	w, err := client.WatchRetryStrategy(c.Context(), tag)
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(w, tc.IsNil)
}
