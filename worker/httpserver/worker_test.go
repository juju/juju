// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/httpserver"
	"github.com/juju/juju/worker/workertest"
)

type workerFixture struct {
	testing.IsolationSuite
	agentConfig          mockAgentConfig
	prometheusRegisterer stubPrometheusRegisterer
	mux                  *apiserverhttp.Mux
	config               httpserver.Config
	stub                 testing.Stub
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agentConfig = mockAgentConfig{
		info: &params.StateServingInfo{
			APIPort: 0, // listen on any port
		},
	}
	certPool, err := api.CreateCertPool(coretesting.CACert)
	c.Assert(err, jc.ErrorIsNil)
	tlsConfig := api.NewTLSConfig(certPool)
	tlsConfig.ServerName = "juju-apiserver"
	tlsConfig.Certificates = []tls.Certificate{*coretesting.ServerTLSCert}
	s.prometheusRegisterer = stubPrometheusRegisterer{}
	s.mux = apiserverhttp.NewMux()

	s.config = httpserver.Config{
		AgentConfig:          &s.agentConfig,
		TLSConfig:            tlsConfig,
		Mux:                  s.mux,
		PrometheusRegisterer: &s.prometheusRegisterer,
	}
}

type WorkerValidationSuite struct {
	workerFixture
}

var _ = gc.Suite(&WorkerValidationSuite{})

func (s *WorkerValidationSuite) TestValidateErrors(c *gc.C) {
	type test struct {
		f      func(*httpserver.Config)
		expect string
	}
	tests := []test{{
		func(cfg *httpserver.Config) { cfg.AgentConfig = nil },
		"nil AgentConfig not valid",
	}, {
		func(cfg *httpserver.Config) { cfg.TLSConfig = nil },
		"nil TLSConfig not valid",
	}, {
		func(cfg *httpserver.Config) { cfg.Mux = nil },
		"nil Mux not valid",
	}, {
		func(cfg *httpserver.Config) { cfg.PrometheusRegisterer = nil },
		"nil PrometheusRegisterer not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerValidationSuite) testValidateError(c *gc.C, f func(*httpserver.Config), expect string) {
	config := s.config
	f(&config)
	w, err := httpserver.NewWorker(config)
	if !c.Check(err, gc.NotNil) {
		workertest.DirtyKill(c, w)
		return
	}
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}

/*
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
	_, err := httpserver.NewWorker(s.config)
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
	_, err := httpserver.NewWorker(s.config)
	c.Check(err, gc.ErrorMatches, "getting log sink config: "+expect)
}
*/

type WorkerSuite struct {
	workerFixture
	worker *httpserver.Worker
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)
	worker, err := httpserver.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
	})
	s.worker = worker
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *WorkerSuite) TestURL(c *gc.C) {
	url := s.worker.URL()
	c.Assert(url, gc.Matches, "https://.*")
}

func (s *WorkerSuite) TestURLWorkerDead(c *gc.C) {
	workertest.CleanKill(c, s.worker)
	url := s.worker.URL()
	c.Assert(url, gc.Matches, "")
}

func (s *WorkerSuite) TestRoundTrip(c *gc.C) {
	s.makeRequest(c, s.worker.URL())
}

func (s *WorkerSuite) makeRequest(c *gc.C, url string) {
	s.mux.AddHandler("GET", "/hello/:name", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "hello, "+r.URL.Query().Get(":name"))
	}))
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: s.config.TLSConfig,
		},
	}
	resp, err := client.Get(url + "/hello/world")
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	out, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, "hello, world")
}

func (s *WorkerSuite) TestWaitsForClients(c *gc.C) {
	// Check that the httpserver stays functional until any clients
	// have finished with it.
	s.mux.AddClient()

	// Get the URL beforehand - we can't get it after the worker has
	// been killed.
	url := s.worker.URL()

	// Shouldn't take effect until the client has done.
	s.worker.Kill()

	waitResult := make(chan error)
	go func() {
		waitResult <- s.worker.Wait()
	}()

	select {
	case <-waitResult:
		c.Fatalf("didn't wait for clients to finish with the mux")
	case <-time.After(coretesting.ShortWait):
	}

	// httpserver is still working.
	s.makeRequest(c, url)

	s.mux.ClientDone()
	select {
	case err := <-waitResult:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("didn't stop after clients were finished")
	}
}

func (s *WorkerSuite) TestMinTLSVersion(c *gc.C) {
	parsed, err := url.Parse(s.worker.URL())
	c.Assert(err, jc.ErrorIsNil)

	tlsConfig := s.config.TLSConfig
	// Specify an unsupported TLS version
	tlsConfig.MaxVersion = tls.VersionSSL30

	conn, err := tls.Dial("tcp", parsed.Host, tlsConfig)
	c.Assert(err, gc.ErrorMatches, ".*protocol version not supported")
	c.Assert(conn, gc.IsNil)
}
