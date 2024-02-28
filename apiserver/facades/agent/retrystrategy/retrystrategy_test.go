// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/retrystrategy"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	jujufactory "github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&retryStrategySuite{})

type retryStrategySuite struct {
	jujutesting.ApiServerSuite

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

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
	s.ApiServerSuite.SetUpTest(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	s.unit = f.MakeUnit(c, nil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.unit.UnitTag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	strategy, err := retrystrategy.NewRetryStrategyAPI(facadetest.ModelContext{
		State_:     s.ControllerModel(c).State(),
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.strategy = strategy
}

func (s *retryStrategySuite) TestRetryStrategyUnauthenticated(c *gc.C) {
	app, err := s.unit.Application()
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	otherUnit := f.MakeUnit(c, &jujufactory.UnitParams{Application: app})
	args := params.Entities{Entities: []params.Entity{{otherUnit.Tag().String()}}}

	res, err := s.strategy.RetryStrategy(context.Background(), args)
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
	res, err := s.strategy.RetryStrategy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, len(tagsTests))
	for i, r := range res.Results {
		c.Logf("result %d", i)
		c.Assert(r.Error, gc.ErrorMatches, tagsTests[i].expectedErr)
		c.Assert(res.Results[i].Result, gc.IsNil)
	}
}

func (s *retryStrategySuite) TestRetryStrategyUnit(c *gc.C) {
	s.assertRetryStrategy(c, s.unit.Tag().String())
}

func (s *retryStrategySuite) TestRetryStrategyApplication(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	app := f.MakeApplication(c, &jujufactory.ApplicationParams{Name: "app"})
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: app.Tag(),
	}

	strategy, err := retrystrategy.NewRetryStrategyAPI(facadetest.ModelContext{
		State_:     s.ControllerModel(c).State(),
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.strategy = strategy

	s.assertRetryStrategy(c, app.Tag().String())
}

func (s *retryStrategySuite) assertRetryStrategy(c *gc.C, tag string) {
	expected := &params.RetryStrategy{
		ShouldRetry:     true,
		MinRetryTime:    retrystrategy.MinRetryTime,
		MaxRetryTime:    retrystrategy.MaxRetryTime,
		JitterRetryTime: retrystrategy.JitterRetryTime,
		RetryTimeFactor: retrystrategy.RetryTimeFactor,
	}
	args := params.Entities{Entities: []params.Entity{{Tag: tag}}}
	r, err := s.strategy.RetryStrategy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Results, gc.HasLen, 1)
	c.Assert(r.Results[0].Error, gc.IsNil)
	c.Assert(r.Results[0].Result, jc.DeepEquals, expected)

	s.setRetryStrategy(c, false)
	expected.ShouldRetry = false

	r, err = s.strategy.RetryStrategy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Results, gc.HasLen, 1)
	c.Assert(r.Results[0].Error, gc.IsNil)
	c.Assert(r.Results[0].Result, jc.DeepEquals, expected)
}

func (s *retryStrategySuite) setRetryStrategy(c *gc.C, automaticallyRetryHooks bool) {
	err := s.ControllerModel(c).UpdateModelConfig(map[string]interface{}{"automatically-retry-hooks": automaticallyRetryHooks}, nil)
	c.Assert(err, jc.ErrorIsNil)
	modelConfig, err := s.ControllerModel(c).ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelConfig.AutomaticallyRetryHooks(), gc.Equals, automaticallyRetryHooks)
}

func (s *retryStrategySuite) TestWatchRetryStrategyUnauthenticated(c *gc.C) {
	app, err := s.unit.Application()
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	otherUnit := f.MakeUnit(c, &jujufactory.UnitParams{Application: app})
	args := params.Entities{Entities: []params.Entity{{otherUnit.Tag().String()}}}

	res, err := s.strategy.WatchRetryStrategy(context.Background(), args)
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
	res, err := s.strategy.WatchRetryStrategy(context.Background(), args)
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
	r, err := s.strategy.WatchRetryStrategy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	wc := statetesting.NewNotifyWatcherC(c, resource.(state.NotifyWatcher))
	wc.AssertNoChange()

	s.setRetryStrategy(c, false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}
