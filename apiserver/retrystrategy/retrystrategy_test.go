// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/retrystrategy"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	jujufactory "github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&retryStrategySuite{})

type retryStrategySuite struct {
	jujutesting.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	factory *jujufactory.Factory

	unit *state.Unit

	strategy retrystrategy.RetryStrategy
}

var tagsTests = []struct {
	tag         string
	expectedErr string
}{
	{"user-admin", "permission denied"},
	{"unit-wut-4", "permission denied"},
	{"definitelynotatag", `"definitelynotatag" is not a valid tag`},
	{"machine-5", "permission denied"},
}

func (s *retryStrategySuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.factory = jujufactory.NewFactory(s.State)
	s.unit = s.factory.MakeUnit(c, nil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.unit.UnitTag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	strategy, err := retrystrategy.NewRetryStrategyAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.strategy = strategy
}

func (s *retryStrategySuite) TestRetryStrategyUnauthenticated(c *gc.C) {
	svc, err := s.unit.Service()
	c.Assert(err, jc.ErrorIsNil)
	otherUnit := s.factory.MakeUnit(c, &jujufactory.UnitParams{Service: svc})
	args := params.Entities{Entities: []params.Entity{{otherUnit.Tag().String()}}}

	res, err := s.strategy.RetryStrategy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, gc.ErrorMatches, "permission denied")
	c.Assert(res.Results[0].Result, gc.IsNil)
}

func (s *retryStrategySuite) TestRetryStrategyBadTag(c *gc.C) {
	args := params.Entities{Entities: make([]params.Entity, len(tagsTests))}
	for i, t := range tagsTests {
		args.Entities[i] = params.Entity{Tag: t.tag}
	}
	res, err := s.strategy.RetryStrategy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, len(tagsTests))
	for i, r := range res.Results {
		c.Logf("result %d", i)
		c.Assert(r.Error, gc.ErrorMatches, tagsTests[i].expectedErr)
		c.Assert(res.Results[i].Result, gc.IsNil)
	}
}

func (s *retryStrategySuite) TestRetryStrategy(c *gc.C) {
	expected := &params.RetryStrategy{
		ShouldRetry:     true,
		MinRetryTime:    retrystrategy.MinRetryTime,
		MaxRetryTime:    retrystrategy.MaxRetryTime,
		JitterRetryTime: retrystrategy.JitterRetryTime,
		RetryTimeFactor: retrystrategy.RetryTimeFactor,
	}
	args := params.Entities{Entities: []params.Entity{{Tag: s.unit.Tag().String()}}}
	r, err := s.strategy.RetryStrategy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Results, gc.HasLen, 1)
	c.Assert(r.Results[0].Error, gc.IsNil)
	c.Assert(r.Results[0].Result, jc.DeepEquals, expected)

	s.setRetryStrategy(c, false)
	expected.ShouldRetry = false

	r, err = s.strategy.RetryStrategy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Results, gc.HasLen, 1)
	c.Assert(r.Results[0].Error, gc.IsNil)
	c.Assert(r.Results[0].Result, jc.DeepEquals, expected)
}

func (s *retryStrategySuite) setRetryStrategy(c *gc.C, automaticallyRetryHooks bool) {
	err := s.State.UpdateModelConfig(map[string]interface{}{"automatically-retry-hooks": automaticallyRetryHooks}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envConfig.AutomaticallyRetryHooks(), gc.Equals, automaticallyRetryHooks)
}

func (s *retryStrategySuite) TestWatchRetryStrategyUnauthenticated(c *gc.C) {
	svc, err := s.unit.Service()
	c.Assert(err, jc.ErrorIsNil)
	otherUnit := s.factory.MakeUnit(c, &jujufactory.UnitParams{Service: svc})
	args := params.Entities{Entities: []params.Entity{{otherUnit.Tag().String()}}}

	res, err := s.strategy.WatchRetryStrategy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, gc.ErrorMatches, "permission denied")
	c.Assert(res.Results[0].NotifyWatcherId, gc.Equals, "")
}

func (s *retryStrategySuite) TestWatchRetryStrategyBadTag(c *gc.C) {
	args := params.Entities{Entities: make([]params.Entity, len(tagsTests))}
	for i, t := range tagsTests {
		args.Entities[i] = params.Entity{Tag: t.tag}
	}
	res, err := s.strategy.WatchRetryStrategy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, len(tagsTests))
	for i, r := range res.Results {
		c.Logf("result %d", i)
		c.Assert(r.Error, gc.ErrorMatches, tagsTests[i].expectedErr)
		c.Assert(res.Results[i].NotifyWatcherId, gc.Equals, "")
	}
}

func (s *retryStrategySuite) TestWatchRetryStrategy(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.unit.UnitTag().String()},
		{Tag: "unit-foo-42"},
	}}
	r, err := s.strategy.WatchRetryStrategy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	wc := statetesting.NewNotifyWatcherC(c, s.State, resource.(state.NotifyWatcher))
	wc.AssertNoChange()

	s.setRetryStrategy(c, false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}
