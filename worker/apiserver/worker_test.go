// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/juju/pubsub"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	coreapiserver "github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/audit"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/apiserver"
	"github.com/juju/juju/worker/workertest"
)

type workerFixture struct {
	testing.IsolationSuite
	agentConfig          mockAgentConfig
	clock                *testing.Clock
	hub                  pubsub.StructuredHub
	prometheusRegisterer stubPrometheusRegisterer
	certWatcher          stubCertWatcher
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
	s.clock = testing.NewClock(time.Time{})
	s.prometheusRegisterer = stubPrometheusRegisterer{}
	s.certWatcher = stubCertWatcher{}
	s.stub.ResetCalls()

	s.config = apiserver.Config{
		AgentConfig:                       &s.agentConfig,
		Clock:                             s.clock,
		Hub:                               &s.hub,
		State:                             &state.State{},
		PrometheusRegisterer:              &s.prometheusRegisterer,
		RegisterIntrospectionHTTPHandlers: func(func(string, http.Handler)) {},
		SetStatePool:                      func(*state.StatePool) {},
		LoginValidator:                    func(names.Tag) error { return nil },
		GetCertificate:                    func() *tls.Certificate { return nil },
		StoreAuditEntry:                   s.storeAuditEntry,
		NewServer:                         s.newServer,
	}
}

func (s *workerFixture) storeAuditEntry(entry audit.AuditEntry) error {
	s.stub.MethodCall(s, "StoreAuditEntry", entry)
	return s.stub.NextErr()
}

func (s *workerFixture) newServer(
	statePool *state.StatePool,
	listener net.Listener,
	config coreapiserver.ServerConfig,
) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewServer", statePool, listener, config)
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
		func(cfg *apiserver.Config) { cfg.Clock = nil },
		"nil Clock not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.Hub = nil },
		"nil Hub not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.State = nil },
		"nil State not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.PrometheusRegisterer = nil },
		"nil PrometheusRegisterer not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.RegisterIntrospectionHTTPHandlers = nil },
		"nil RegisterIntrospectionHTTPHandlers not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.SetStatePool = nil },
		"nil SetStatePool not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.LoginValidator = nil },
		"nil LoginValidator not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.GetCertificate = nil },
		"nil GetCertificate not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.StoreAuditEntry = nil },
		"nil StoreAuditEntry not valid",
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

func (s *WorkerValidationSuite) TestMissingStateServingInfo(c *gc.C) {
	s.agentConfig.info = nil
	_, err := apiserver.NewWorker(s.config)
	c.Assert(err, gc.ErrorMatches, "missing state serving info")
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
