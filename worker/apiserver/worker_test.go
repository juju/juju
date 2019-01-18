// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"net/http"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/agent"
	coreapiserver "github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/apiserver"
)

type workerFixture struct {
	testing.IsolationSuite
	agentConfig          mockAgentConfig
	authenticator        *mockAuthenticator
	clock                *testclock.Clock
	controller           *cache.Controller
	hub                  pubsub.StructuredHub
	mux                  *apiserverhttp.Mux
	prometheusRegisterer stubPrometheusRegisterer
	leaseManager         lease.Manager
	config               apiserver.Config
	stub                 testing.Stub
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agentConfig = mockAgentConfig{
		dataDir: c.MkDir(),
		logDir:  c.MkDir(),
		info: &params.StateServingInfo{
			APIPort: 0, // listen on any port
		},
	}
	s.authenticator = &mockAuthenticator{}
	s.clock = testclock.NewClock(time.Time{})
	controller, err := cache.NewController(cache.ControllerConfig{
		Changes: make(chan interface{}),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.controller = controller
	s.mux = apiserverhttp.NewMux()
	s.prometheusRegisterer = stubPrometheusRegisterer{}
	s.leaseManager = &struct{ lease.Manager }{}
	s.stub.ResetCalls()

	s.config = apiserver.Config{
		AgentConfig:                       &s.agentConfig,
		Authenticator:                     s.authenticator,
		Clock:                             s.clock,
		Controller:                        s.controller,
		Hub:                               &s.hub,
		Presence:                          presence.New(s.clock),
		Mux:                               s.mux,
		StatePool:                         &state.StatePool{},
		PrometheusRegisterer:              &s.prometheusRegisterer,
		LeaseManager:                      s.leaseManager,
		RegisterIntrospectionHTTPHandlers: func(func(string, http.Handler)) {},
		UpgradeComplete:                   func() bool { return true },
		RestoreStatus:                     func() state.RestoreStatus { return "" },
		NewServer:                         s.newServer,
	}
}

func (s *workerFixture) newServer(config coreapiserver.ServerConfig) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewServer", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

type WorkerValidationSuite struct {
	workerFixture
}

var _ = gc.Suite(&WorkerValidationSuite{})

func (s *WorkerValidationSuite) TestValidateErrors(c *gc.C) {
	type test struct {
		f      func(*apiserver.Config)
		expect string
	}
	tests := []test{{
		func(cfg *apiserver.Config) { cfg.AgentConfig = nil },
		"nil AgentConfig not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.Authenticator = nil },
		"nil Authenticator not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.Clock = nil },
		"nil Clock not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.Hub = nil },
		"nil Hub not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.Mux = nil },
		"nil Mux not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.StatePool = nil },
		"nil StatePool not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.PrometheusRegisterer = nil },
		"nil PrometheusRegisterer not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.LeaseManager = nil },
		"nil LeaseManager not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.RegisterIntrospectionHTTPHandlers = nil },
		"nil RegisterIntrospectionHTTPHandlers not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.UpgradeComplete = nil },
		"nil UpgradeComplete not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.RestoreStatus = nil },
		"nil RestoreStatus not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.NewServer = nil },
		"nil NewServer not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerValidationSuite) testValidateError(c *gc.C, f func(*apiserver.Config), expect string) {
	config := s.config
	f(&config)
	w, err := apiserver.NewWorker(config)
	if !c.Check(err, gc.NotNil) {
		workertest.DirtyKill(c, w)
		return
	}
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *WorkerValidationSuite) TestValidateRateLimitConfig(c *gc.C) {
	s.testValidateRateLimitConfig(c, agent.AgentLoginRateLimit, "foo", "parsing AGENT_LOGIN_RATE_LIMIT: .*")
	s.testValidateRateLimitConfig(c, agent.AgentLoginMinPause, "foo", "parsing AGENT_LOGIN_MIN_PAUSE: .*")
	s.testValidateRateLimitConfig(c, agent.AgentLoginMaxPause, "foo", "parsing AGENT_LOGIN_MAX_PAUSE: .*")
	s.testValidateRateLimitConfig(c, agent.AgentLoginRetryPause, "foo", "parsing AGENT_LOGIN_RETRY_PAUSE: .*")
	s.testValidateRateLimitConfig(c, agent.AgentConnMinPause, "foo", "parsing AGENT_CONN_MIN_PAUSE: .*")
	s.testValidateRateLimitConfig(c, agent.AgentConnMaxPause, "foo", "parsing AGENT_CONN_MAX_PAUSE: .*")
	s.testValidateRateLimitConfig(c, agent.AgentConnLookbackWindow, "foo", "parsing AGENT_CONN_LOOKBACK_WINDOW: .*")
	s.testValidateRateLimitConfig(c, agent.AgentConnLowerThreshold, "foo", "parsing AGENT_CONN_LOWER_THRESHOLD: .*")
	s.testValidateRateLimitConfig(c, agent.AgentConnUpperThreshold, "foo", "parsing AGENT_CONN_UPPER_THRESHOLD: .*")
}

func (s *WorkerValidationSuite) testValidateRateLimitConfig(c *gc.C, key, value, expect string) {
	s.agentConfig.values = map[string]string{key: value}
	_, err := apiserver.NewWorker(s.config)
	c.Check(err, gc.ErrorMatches, "getting rate limit config: "+expect)
}

func (s *WorkerValidationSuite) TestValidateLogSinkConfig(c *gc.C) {
	s.testValidateLogSinkConfig(c, agent.LogSinkDBLoggerBufferSize, "foo", "parsing LOGSINK_DBLOGGER_BUFFER_SIZE: .*")
	s.testValidateLogSinkConfig(c, agent.LogSinkDBLoggerFlushInterval, "foo", "parsing LOGSINK_DBLOGGER_FLUSH_INTERVAL: .*")
	s.testValidateLogSinkConfig(c, agent.LogSinkRateLimitBurst, "foo", "parsing LOGSINK_RATELIMIT_BURST: .*")
	s.testValidateLogSinkConfig(c, agent.LogSinkRateLimitRefill, "foo", "parsing LOGSINK_RATELIMIT_REFILL: .*")
}

func (s *WorkerValidationSuite) testValidateLogSinkConfig(c *gc.C, key, value, expect string) {
	s.agentConfig.values = map[string]string{key: value}
	_, err := apiserver.NewWorker(s.config)
	c.Check(err, gc.ErrorMatches, "getting log sink config: "+expect)
}
