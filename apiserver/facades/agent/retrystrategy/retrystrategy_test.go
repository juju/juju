// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/retrystrategy"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

func TestRetryStrategySuite(t *testing.T) {
	tc.Run(t, &retryStrategySuite{})
}

type retryStrategySuite struct {
	strategy           retrystrategy.RetryStrategy
	authorizer         apiservertesting.FakeAuthorizer
	modelConfigService *MockModelConfigService
	watcherRegistry    *facademocks.MockWatcherRegistry
}

var tagsTests = []struct {
	tag         string
	expectedErr string
}{
	{tag: "user-admin", expectedErr: "permission denied"},
	{tag: "unit-wut-4", expectedErr: "permission denied"},
	{tag: "definitelynotatag", expectedErr: `"definitelynotatag" is not a valid tag`},
	{tag: "machine-5", expectedErr: "permission denied"},
}

func (s *retryStrategySuite) SetUpTest(c *tc.C) {
	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("mysql/0"),
	}
}

func (s *retryStrategySuite) setupAPI(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	strategy, err := retrystrategy.NewRetryStrategyAPI(
		s.authorizer,
		s.modelConfigService,
		s.watcherRegistry,
	)
	c.Assert(err, tc.ErrorIsNil)
	s.strategy = strategy

	return ctrl
}

func (s *retryStrategySuite) TestRetryStrategyUnauthenticated(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	args := params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-1"}}}

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(
		config.New(false, map[string]any{
			"name":                         "donotuse",
			"type":                         "donotuse",
			"uuid":                         "00000000-0000-0000-0000-000000000000",
			config.AutomaticallyRetryHooks: true,
		}),
	)
	res, err := s.strategy.RetryStrategy(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.ErrorMatches, "permission denied")
	c.Assert(res.Results[0].Result, tc.IsNil)
}

func (s *retryStrategySuite) TestRetryStrategyBadTag(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	args := params.Entities{Entities: make([]params.Entity, len(tagsTests))}
	for i, t := range tagsTests {
		args.Entities[i] = params.Entity{Tag: t.tag}
	}

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(
		config.New(false, map[string]any{
			"name":                         "donotuse",
			"type":                         "donotuse",
			"uuid":                         "00000000-0000-0000-0000-000000000000",
			config.AutomaticallyRetryHooks: true,
		}),
	)
	res, err := s.strategy.RetryStrategy(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, len(tagsTests))
	for i, r := range res.Results {
		c.Logf("result %d", i)
		c.Assert(r.Error, tc.ErrorMatches, tagsTests[i].expectedErr)
		c.Assert(res.Results[i].Result, tc.IsNil)
	}
}

func (s *retryStrategySuite) TestRetryStrategyUnit(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.assertRetryStrategy(c, "unit-mysql-0")
}

func (s *retryStrategySuite) TestRetryStrategyApplication(c *tc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("app"),
	}
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.assertRetryStrategy(c, "application-app")
}

func (s *retryStrategySuite) assertRetryStrategy(c *tc.C, tag string) {
	expected := &params.RetryStrategy{
		ShouldRetry:     true,
		MinRetryTime:    retrystrategy.MinRetryTime,
		MaxRetryTime:    retrystrategy.MaxRetryTime,
		JitterRetryTime: retrystrategy.JitterRetryTime,
		RetryTimeFactor: retrystrategy.RetryTimeFactor,
	}
	args := params.Entities{Entities: []params.Entity{{Tag: tag}}}

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(
		config.New(false, map[string]any{
			"name":                         "donotuse",
			"type":                         "donotuse",
			"uuid":                         "00000000-0000-0000-0000-000000000000",
			config.AutomaticallyRetryHooks: true,
		}),
	)
	r, err := s.strategy.RetryStrategy(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Results, tc.HasLen, 1)
	c.Assert(r.Results[0].Error, tc.IsNil)
	c.Assert(r.Results[0].Result, tc.DeepEquals, expected)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(
		config.New(false, map[string]any{
			"name":                         "donotuse",
			"type":                         "donotuse",
			"uuid":                         "00000000-0000-0000-0000-000000000000",
			config.AutomaticallyRetryHooks: false,
		}),
	)
	expected.ShouldRetry = false

	r, err = s.strategy.RetryStrategy(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Results, tc.HasLen, 1)
	c.Assert(r.Results[0].Error, tc.IsNil)
	c.Assert(r.Results[0].Result, tc.DeepEquals, expected)
}

func (s *retryStrategySuite) TestWatchRetryStrategyUnauthenticated(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	args := params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-1"}}}
	res, err := s.strategy.WatchRetryStrategy(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.ErrorMatches, "permission denied")
	c.Assert(res.Results[0].NotifyWatcherId, tc.Equals, "")
}

func (s *retryStrategySuite) TestWatchRetryStrategyBadTag(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	args := params.Entities{Entities: make([]params.Entity, len(tagsTests))}
	for i, t := range tagsTests {
		args.Entities[i] = params.Entity{Tag: t.tag}
	}
	res, err := s.strategy.WatchRetryStrategy(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, len(tagsTests))
	for i, r := range res.Results {
		c.Logf("result %d", i)
		c.Assert(r.Error, tc.ErrorMatches, tagsTests[i].expectedErr)
		c.Assert(res.Results[i].NotifyWatcherId, tc.Equals, "")
	}
}

func (s *retryStrategySuite) TestWatchRetryStrategy(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	notifyCh := make(chan []string, 1)
	notifyCh <- []string{}
	watcher := watchertest.NewMockStringsWatcher(notifyCh)
	s.modelConfigService.EXPECT().Watch(gomock.Any()).Return(watcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-foo-42"},
	}}
	r, err := s.strategy.WatchRetryStrategy(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}
