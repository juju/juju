// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"net/http"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/agent"
	coreapiserver "github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/lease"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/internal/jwtparser"
	"github.com/juju/juju/internal/services"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apiserver"
	"github.com/juju/juju/state"
)

type workerFixture struct {
	testing.IsolationSuite
	agentConfig             mockAgentConfig
	authenticator           *mockAuthenticator
	clock                   *testclock.Clock
	hub                     pubsub.StructuredHub
	mux                     *apiserverhttp.Mux
	prometheusRegisterer    stubPrometheusRegisterer
	leaseManager            lease.Manager
	config                  apiserver.Config
	stub                    testing.Stub
	metricsCollector        *coreapiserver.Collector
	logSink                 corelogger.ModelLogger
	charmhubHTTPClient      *http.Client
	dbGetter                stubWatchableDBGetter
	dbDeleter               stubDBDeleter
	tracerGetter            stubTracerGetter
	objectStoreGetter       stubObjectStoreGetter
	controllerConfigService *MockControllerConfigService
	modelService            *MockModelService
	domainServicesGetter    services.DomainServicesGetter
	controllerUUID          string
	controllerModelUUID     model.UUID
	jwtParser               *jwtparser.Parser
}

func (s *workerFixture) SetUpTest(c *tc.C) {
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
	s.logSink = &mockModelLogger{}
	s.charmhubHTTPClient = &http.Client{}
	s.domainServicesGetter = &stubDomainServicesGetter{}
	s.controllerUUID = coretesting.ControllerTag.Id()
	s.controllerModelUUID = modeltesting.GenModelUUID(c)
	s.stub.ResetCalls()
	s.jwtParser = &jwtparser.Parser{}

	s.config = apiserver.Config{
		AgentConfig:                       &s.agentConfig,
		LocalMacaroonAuthenticator:        s.authenticator,
		Clock:                             s.clock,
		Hub:                               &s.hub,
		Mux:                               s.mux,
		StatePool:                         &state.StatePool{},
		LeaseManager:                      s.leaseManager,
		RegisterIntrospectionHTTPHandlers: func(func(string, http.Handler)) {},
		UpgradeComplete:                   func() bool { return true },
		NewServer:                         s.newServer,
		MetricsCollector:                  s.metricsCollector,
		LogSink:                           s.logSink,
		CharmhubHTTPClient:                s.charmhubHTTPClient,
		DBGetter:                          s.dbGetter,
		DBDeleter:                         s.dbDeleter,
		ControllerConfigService:           s.controllerConfigService,
		ModelService:                      s.modelService,
		DomainServicesGetter:              s.domainServicesGetter,
		TracerGetter:                      s.tracerGetter,
		ObjectStoreGetter:                 s.objectStoreGetter,
		JWTParser:                         s.jwtParser,
	}
}

func (s *workerFixture) newServer(ctx context.Context, config coreapiserver.ServerConfig) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewServer", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

type WorkerValidationSuite struct {
	workerFixture
}

var _ = tc.Suite(&WorkerValidationSuite{})

func (s *WorkerValidationSuite) TestValidateErrors(c *tc.C) {
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
		func(cfg *apiserver.Config) { cfg.LogSink = nil },
		"nil LogSink not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.DBGetter = nil },
		"nil DBGetter not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.DomainServicesGetter = nil },
		"nil DomainServicesGetter not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.TracerGetter = nil },
		"nil TracerGetter not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.ObjectStoreGetter = nil },
		"nil ObjectStoreGetter not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.ControllerConfigService = nil },
		"nil ControllerConfigService not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.ModelService = nil },
		"nil ModelService not valid",
	}, {
		func(cfg *apiserver.Config) { cfg.JWTParser = nil },
		"nil JWTParser not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerValidationSuite) testValidateError(c *tc.C, f func(*apiserver.Config), expect string) {
	config := s.config
	f(&config)
	w, err := apiserver.NewWorker(context.Background(), config)
	if !c.Check(err, tc.NotNil) {
		workertest.DirtyKill(c, w)
		return
	}
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, expect)
}

func (s *WorkerValidationSuite) TestValidateLogSinkConfig(c *tc.C) {
	s.testValidateLogSinkConfig(c, agent.LogSinkRateLimitBurst, "foo", "parsing LOGSINK_RATELIMIT_BURST: .*")
	s.testValidateLogSinkConfig(c, agent.LogSinkRateLimitRefill, "foo", "parsing LOGSINK_RATELIMIT_REFILL: .*")
}

func (s *WorkerValidationSuite) testValidateLogSinkConfig(c *tc.C, key, value, expect string) {
	s.agentConfig.values = map[string]string{key: value}
	_, err := apiserver.NewWorker(context.Background(), s.config)
	c.Check(err, tc.ErrorMatches, "getting log sink config: "+expect)
}
