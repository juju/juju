// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/logger"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

var (
	defaultMachineTag = names.NewMachineTag("0")
)

type loggerSuite struct {
	logger     *logger.LoggerAPI
	authorizer apiservertesting.FakeAuthorizer

	watcherRegistry *facademocks.MockWatcherRegistry

	modelConfigService *MockModelConfigService
}

var _ = tc.Suite(&loggerSuite{})

func (s *loggerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	s.modelConfigService = NewMockModelConfigService(ctrl)

	return ctrl
}

func (s *loggerSuite) setupAPI(c *tc.C) {
	var err error
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: defaultMachineTag,
	}
	s.logger, err = logger.NewLoggerAPI(s.authorizer, s.watcherRegistry, s.modelConfigService)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *loggerSuite) TestNewLoggerAPIRefusesNonAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// We aren't even a machine agent
	var err error
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("some-user"),
	}
	s.logger, err = logger.NewLoggerAPI(s.authorizer, s.watcherRegistry, s.modelConfigService)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *loggerSuite) TestNewLoggerAPIAcceptsUnitAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	var err error
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("germany/7"),
	}
	s.logger, err = logger.NewLoggerAPI(s.authorizer, s.watcherRegistry, s.modelConfigService)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *loggerSuite) TestNewLoggerAPIAcceptsApplicationAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	var err error
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("germany"),
	}
	s.logger, err = logger.NewLoggerAPI(s.authorizer, s.watcherRegistry, s.modelConfigService)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *loggerSuite) TestWatchLoggingConfigNothing(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)
	// Not an error to watch nothing
	results := s.logger.WatchLoggingConfig(c.Context(), params.Entities{})
	c.Assert(results.Results, tc.HasLen, 0)
}

func (s *loggerSuite) TestWatchLoggingConfig(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	notifyCh := make(chan []string, 1)
	notifyCh <- []string{}
	watcher := watchertest.NewMockStringsWatcher(notifyCh)
	s.modelConfigService.EXPECT().Watch().Return(watcher, nil)

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	args := params.Entities{
		Entities: []params.Entity{{Tag: defaultMachineTag.String()}},
	}
	results := s.logger.WatchLoggingConfig(c.Context(), args)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].NotifyWatcherId, tc.Not(tc.Equals), "")
	c.Assert(results.Results[0].Error, tc.IsNil)
}

func (s *loggerSuite) TestWatchLoggingConfigRefusesWrongAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)
	// We are a machine agent, but not the one we are trying to track
	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-12354"}},
	}
	results := s.logger.WatchLoggingConfig(c.Context(), args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].NotifyWatcherId, tc.Equals, "")
	c.Assert(results.Results[0].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *loggerSuite) TestLoggingConfigForNoone(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)
	// Not an error to request nothing, dumb, but not an error.
	results := s.logger.LoggingConfig(c.Context(), params.Entities{})
	c.Assert(results.Results, tc.HasLen, 0)
}

func (s *loggerSuite) TestLoggingConfigRefusesWrongAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(nil, nil)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-12354"}},
	}
	results := s.logger.LoggingConfig(c.Context(), args)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *loggerSuite) TestLoggingConfigForAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	cfg, err := config.New(false, map[string]any{
		"name": "donotuse",
		"type": "donotuse",
		"uuid": "00000000-0000-0000-0000-000000000000",

		"logging-config": "<root>=WARN;juju.log.test=DEBUG;unit=INFO",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)

	args := params.Entities{
		Entities: []params.Entity{{Tag: defaultMachineTag.String()}},
	}
	results := s.logger.LoggingConfig(c.Context(), args)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.Result, tc.Equals, "<root>=WARN;juju.log.test=DEBUG;unit=INFO")
}
