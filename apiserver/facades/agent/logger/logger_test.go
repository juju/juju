// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"context"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/logger"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/logging"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

var (
	defaultMachineTag = names.NewMachineTag("0")
)

type loggerSuite struct {
	logger     *logger.LoggerAPI
	loggerV2   *logger.LoggerAPIV2
	authorizer apiservertesting.FakeAuthorizer

	watcherRegistry *facademocks.MockWatcherRegistry

	modelConfigService          *MockModelConfigService
	controllerLokiConfigService *stubControllerLokiConfigService
}

func TestLoggerSuite(t *testing.T) {
	tc.Run(t, &loggerSuite{})
}

func (s *loggerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.controllerLokiConfigService = &stubControllerLokiConfigService{}

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

func (s *loggerSuite) setupAPIV2(c *tc.C) {
	var err error
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: defaultMachineTag,
	}
	s.loggerV2, err = logger.NewLoggerAPIV2(
		s.authorizer,
		s.watcherRegistry,
		s.modelConfigService,
		s.controllerLokiConfigService,
	)
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
	s.modelConfigService.EXPECT().Watch(gomock.Any()).Return(watcher, nil)

	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("1", nil)

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

func (s *loggerSuite) TestGetControllerLokiConfigForNoone(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPIV2(c)

	results := s.loggerV2.GetControllerLokiConfig(c.Context(), params.Entities{})
	c.Assert(results.Results, tc.HasLen, 0)
	c.Check(s.controllerLokiConfigService.getCalls, tc.Equals, 0)
}

func (s *loggerSuite) TestGetControllerLokiConfigRefusesWrongAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPIV2(c)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-12354"}},
	}
	results := s.loggerV2.GetControllerLokiConfig(c.Context(), args)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Check(s.controllerLokiConfigService.getCalls, tc.Equals, 0)
}

func (s *loggerSuite) TestGetControllerLokiConfigForAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPIV2(c)

	s.controllerLokiConfigService.config = logging.LokiConfig{
		Endpoint:      "https://loki.example.com/loki/api/v1/push",
		CACertificate: "ca-cert",
	}

	args := params.Entities{
		Entities: []params.Entity{{Tag: defaultMachineTag.String()}},
	}
	results := s.loggerV2.GetControllerLokiConfig(c.Context(), args)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Check(result.Endpoint, tc.Equals, "https://loki.example.com/loki/api/v1/push")
	c.Assert(result.CACert, tc.NotNil)
	c.Check(*result.CACert, tc.Equals, "ca-cert")
	c.Check(s.controllerLokiConfigService.getCalls, tc.Equals, 1)
}

func (s *loggerSuite) TestWatchControllerLokiConfigNothing(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPIV2(c)

	results := s.loggerV2.WatchControllerLokiConfig(c.Context(), params.Entities{})
	c.Assert(results.Results, tc.HasLen, 0)
	c.Check(s.controllerLokiConfigService.watchCalls, tc.Equals, 0)
}

func (s *loggerSuite) TestWatchControllerLokiConfig(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPIV2(c)

	notifyCh := make(chan struct{}, 1)
	notifyCh <- struct{}{}
	s.controllerLokiConfigService.watcher = watchertest.NewMockNotifyWatcher(notifyCh)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("1", nil)

	args := params.Entities{
		Entities: []params.Entity{{Tag: defaultMachineTag.String()}},
	}
	results := s.loggerV2.WatchControllerLokiConfig(c.Context(), args)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].NotifyWatcherId, tc.Not(tc.Equals), "")
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Check(s.controllerLokiConfigService.watchCalls, tc.Equals, 1)
}

func (s *loggerSuite) TestWatchControllerLokiConfigRefusesWrongAgent(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPIV2(c)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-12354"}},
	}
	results := s.loggerV2.WatchControllerLokiConfig(c.Context(), args)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].NotifyWatcherId, tc.Equals, "")
	c.Assert(results.Results[0].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Check(s.controllerLokiConfigService.watchCalls, tc.Equals, 0)
}

type stubControllerLokiConfigService struct {
	config     logging.LokiConfig
	getErr     error
	getCalls   int
	watcher    *watchertest.MockNotifyWatcher
	watchErr   error
	watchCalls int
}

func (s *stubControllerLokiConfigService) GetLokiConfig(ctx context.Context) (logging.LokiConfig, error) {
	s.getCalls++
	return s.config, s.getErr
}

func (s *stubControllerLokiConfigService) WatchLokiConfig(ctx context.Context) (watcher.NotifyWatcher, error) {
	s.watchCalls++
	return s.watcher, s.watchErr
}
