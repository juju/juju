// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"net/http"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/pubsub/v2"
	"github.com/juju/testing"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	coreapiserver "github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/internal/worker/apiserver"
	"github.com/juju/juju/internal/worker/syslogger"
	"github.com/juju/juju/state"
)

type workerFixture struct {
	testing.IsolationSuite
	agentConfig          mockAgentConfig
	authenticator        *mockAuthenticator
	clock                *testclock.Clock
	hub                  pubsub.StructuredHub
	mux                  *apiserverhttp.Mux
	prometheusRegisterer stubPrometheusRegisterer
	leaseManager         lease.Manager
	config               apiserver.Config
	stub                 testing.Stub
	metricsCollector     *coreapiserver.Collector
	sysLogger            syslogger.SysLogger
	charmhubHTTPClient   *http.Client
	dbGetter             stubWatchableDBGetter
	serviceFactoryGetter stubServiceFactoryGetter
	tracerGetter         stubTracerGetter
	objectStoreGetter    stubObjectStoreGetter
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agentConfig = mockAgentConfig{
		dataDir: c.MkDir(),
		logDir:  c.MkDir(),
		info: &controller.StateServingInfo{
			APIPort: 0, // listen on any port
		},
	}
	s.authenticator = &mockAuthenticator{}
	s.clock = testclock.NewClock(time.Time{})
	s.mux = apiserverhttp.NewMux()
	s.prometheusRegisterer = stubPrometheusRegisterer{}
	s.leaseManager = &struct{ lease.Manager }{}
	s.metricsCollector = coreapiserver.NewMetricsCollector()
	s.sysLogger = &mockSysLogger{}
	s.charmhubHTTPClient = &http.Client{}
	s.stub.ResetCalls()

	s.config = apiserver.Config{
		AgentConfig:                       &s.agentConfig,
		LocalMacaroonAuthenticator:        s.authenticator,
		Clock:                             s.clock,
		Hub:                               &s.hub,
		Presence:                          presence.New(s.clock),
		Mux:                               s.mux,
		StatePool:                         &state.StatePool{},
		LeaseManager:                      s.leaseManager,
		RegisterIntrospectionHTTPHandlers: func(func(string, http.Handler)) {},
		UpgradeComplete:                   func() bool { return true },
		NewServer:                         s.newServer,
		MetricsCollector:                  s.metricsCollector,
		SysLogger:                         s.sysLogger,
		CharmhubHTTPClient:                s.charmhubHTTPClient,
		DBGetter:                          s.dbGetter,
		ServiceFactoryGetter:              s.serviceFactoryGetter,
		TracerGetter:                      s.tracerGetter,
		ObjectStoreGetter:                 s.objectStoreGetter,
	}
}

func (s *workerFixture) newServer(ctx context.Context, config coreapiserver.ServerConfig) (worker.Worker, error) {
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
		func(cfg *apiserver.Config) { cfg.LocalMacaroonAuthenticator = nil },
		"nil LocalMacaroonAuthenticator not valid",
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
		func(cfg *apiserver.Config) { cfg.MetricsCollector = nil },
		"nil MetricsCollector not valid",
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
		func(cfg *apiserver.Config) { cfg.NewServer = nil },
		"nil NewServer not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.SysLogger = nil },
		"nil SysLogger not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.DBGetter = nil },
		"nil DBGetter not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.ServiceFactoryGetter = nil },
		"nil ServiceFactoryGetter not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.TracerGetter = nil },
		"nil TracerGetter not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.ObjectStoreGetter = nil },
		"nil ObjectStoreGetter not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerValidationSuite) testValidateError(c *gc.C, f func(*apiserver.Config), expect string) {
	config := s.config
	f(&config)
	w, err := apiserver.NewWorker(context.Background(), config)
	if !c.Check(err, gc.NotNil) {
		workertest.DirtyKill(c, w)
		return
	}
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *WorkerValidationSuite) TestValidateLogSinkConfig(c *gc.C) {
	s.testValidateLogSinkConfig(c, agent.LogSinkDBLoggerBufferSize, "foo", "parsing LOGSINK_DBLOGGER_BUFFER_SIZE: .*")
	s.testValidateLogSinkConfig(c, agent.LogSinkDBLoggerFlushInterval, "foo", "parsing LOGSINK_DBLOGGER_FLUSH_INTERVAL: .*")
	s.testValidateLogSinkConfig(c, agent.LogSinkRateLimitBurst, "foo", "parsing LOGSINK_RATELIMIT_BURST: .*")
	s.testValidateLogSinkConfig(c, agent.LogSinkRateLimitRefill, "foo", "parsing LOGSINK_RATELIMIT_REFILL: .*")
}

func (s *WorkerValidationSuite) testValidateLogSinkConfig(c *gc.C, key, value, expect string) {
	s.agentConfig.values = map[string]string{key: value}
	_, err := apiserver.NewWorker(context.Background(), s.config)
	c.Check(err, gc.ErrorMatches, "getting log sink config: "+expect)
}
