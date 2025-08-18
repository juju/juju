// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
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
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apiserver"
	"github.com/juju/juju/internal/worker/watcherregistry"
)

type workerFixture struct {
	testhelpers.IsolationSuite
	agentConfig             mockAgentConfig
	authenticator           *mockAuthenticator
	clock                   *testclock.Clock
	mux                     *apiserverhttp.Mux
	prometheusRegisterer    stubPrometheusRegisterer
	leaseManager            lease.Manager
	config                  apiserver.Config
	stub                    testhelpers.Stub
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
	watcherRegistryGetter   watcherregistry.WatcherRegistryGetter
	controllerUUID          string
	controllerModelUUID     model.UUID
	jwtParser               *jwtparser.Parser
}

func (s *workerFixture) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.agentConfig = mockAgentConfig{
		dataDir: c.MkDir(),
		logDir:  c.MkDir(),
		info: &controller.ControllerAgentInfo{
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
	s.watcherRegistryGetter = &stubWatcherRegistryGetter{}

	s.config = apiserver.Config{
		AgentConfig:                       &s.agentConfig,
		LocalMacaroonAuthenticator:        s.authenticator,
		Clock:                             s.clock,
		Mux:                               s.mux,
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
		WatcherRegistryGetter:             s.watcherRegistryGetter,
	}
}

func (s *workerFixture) newServer(ctx context.Context, config coreapiserver.ServerConfig) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewServer", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	w, err := worker.NewRunner(worker.RunnerParams{
		Name: "apiserver",
	})
	if err != nil {
		return nil, err
	}
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

type WorkerValidationSuite struct {
	workerFixture
}

func TestWorkerValidationSuite(t *testing.T) {
	tc.Run(t, &WorkerValidationSuite{})
}

func (s *WorkerValidationSuite) TestValidateErrors(c *tc.C) {
	type test struct {
		f      func(*apiserver.Config)
		expect string
	}
	tests := []test{{
		f:      func(cfg *apiserver.Config) { cfg.AgentConfig = nil },
		expect: "nil AgentConfig not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.LocalMacaroonAuthenticator = nil },
		expect: "nil LocalMacaroonAuthenticator not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.Clock = nil },
		expect: "nil Clock not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.Mux = nil },
		expect: "nil Mux not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.MetricsCollector = nil },
		expect: "nil MetricsCollector not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.LeaseManager = nil },
		expect: "nil LeaseManager not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.RegisterIntrospectionHTTPHandlers = nil },
		expect: "nil RegisterIntrospectionHTTPHandlers not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.UpgradeComplete = nil },
		expect: "nil UpgradeComplete not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.NewServer = nil },
		expect: "nil NewServer not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.LogSink = nil },
		expect: "nil LogSink not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.DBGetter = nil },
		expect: "nil DBGetter not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.DomainServicesGetter = nil },
		expect: "nil DomainServicesGetter not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.TracerGetter = nil },
		expect: "nil TracerGetter not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.ObjectStoreGetter = nil },
		expect: "nil ObjectStoreGetter not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.ControllerConfigService = nil },
		expect: "nil ControllerConfigService not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.ModelService = nil },
		expect: "nil ModelService not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.JWTParser = nil },
		expect: "nil JWTParser not valid",
	}, {
		f:      func(cfg *apiserver.Config) { cfg.WatcherRegistryGetter = nil },
		expect: "nil WatcherRegistryGetter not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerValidationSuite) testValidateError(c *tc.C, f func(*apiserver.Config), expect string) {
	config := s.config
	f(&config)
	w, err := apiserver.NewWorker(c.Context(), config)
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
	_, err := apiserver.NewWorker(c.Context(), s.config)
	c.Check(err, tc.ErrorMatches, "getting log sink config: "+expect)
}
