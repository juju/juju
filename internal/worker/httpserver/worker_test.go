// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"syscall"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/httpserver"
)

type workerFixture struct {
	testhelpers.IsolationSuite
	prometheusRegisterer stubPrometheusRegisterer
	agentName            string
	mux                  *apiserverhttp.Mux
	clock                *testclock.Clock
	config               httpserver.Config
	logDir               string
}

func (s *workerFixture) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	certPool, err := api.CreateCertPool(coretesting.CACert)
	c.Assert(err, tc.ErrorIsNil)

	tlsConfig := api.NewTLSConfig(certPool)
	tlsConfig.ServerName = "juju-apiserver"
	tlsConfig.Certificates = []tls.Certificate{*coretesting.ServerTLSCert}

	s.prometheusRegisterer = stubPrometheusRegisterer{}

	s.mux = apiserverhttp.NewMux()
	s.clock = testclock.NewClock(time.Now())

	s.agentName = "machine-42"
	s.logDir = c.MkDir()

	s.config = httpserver.Config{
		AgentName:            s.agentName,
		Clock:                s.clock,
		TLSConfig:            tlsConfig,
		Mux:                  s.mux,
		PrometheusRegisterer: &s.prometheusRegisterer,
		LogDir:               s.logDir,
		MuxShutdownWait:      1 * time.Minute,
		APIPort:              0,
		Logger:               loggertesting.WrapCheckLog(c),
	}
}

type WorkerValidationSuite struct {
	workerFixture
}

func TestWorkerValidationSuite(t *testing.T) {
	tc.Run(t, &WorkerValidationSuite{})
}

func (s *WorkerValidationSuite) TestValidateErrors(c *tc.C) {
	type test struct {
		f      func(*httpserver.Config)
		expect string
	}
	tests := []test{{
		f:      func(cfg *httpserver.Config) { cfg.AgentName = "" },
		expect: "empty AgentName not valid",
	}, {
		f:      func(cfg *httpserver.Config) { cfg.TLSConfig = nil },
		expect: "nil TLSConfig not valid",
	}, {
		f:      func(cfg *httpserver.Config) { cfg.Mux = nil },
		expect: "nil Mux not valid",
	}, {
		f:      func(cfg *httpserver.Config) { cfg.PrometheusRegisterer = nil },
		expect: "nil PrometheusRegisterer not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerValidationSuite) testValidateError(c *tc.C, f func(*httpserver.Config), expect string) {
	config := s.config
	f(&config)
	w, err := httpserver.NewWorker(config)
	if !c.Check(err, tc.NotNil) {
		workertest.DirtyKill(c, w)
		return
	}
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, expect)
}

type WorkerSuite struct {
	workerFixture
	worker *httpserver.Worker
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &WorkerSuite{})
}

func (s *WorkerSuite) SetUpTest(c *tc.C) {
	s.workerFixture.SetUpTest(c)
	worker, err := httpserver.NewWorker(s.config)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) {
		workertest.DirtyKill(c, worker)
	})
	s.worker = worker
}

func (s *WorkerSuite) TestStartStop(c *tc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *WorkerSuite) TestURL(c *tc.C) {
	url := s.worker.URL()
	c.Assert(url, tc.Matches, "https://.*")
}

func (s *WorkerSuite) TestURLWorkerDead(c *tc.C) {
	workertest.CleanKill(c, s.worker)

	s.mux.Wait()

	url := s.worker.URL()
	c.Assert(url, tc.Matches, "")
}

func (s *WorkerSuite) TestRoundTrip(c *tc.C) {
	s.makeRequest(c, s.worker.URL())
}

func (s *WorkerSuite) makeRequest(c *tc.C, url string) {
	s.mux.AddHandler("GET", "/hello/:name", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "hello, "+r.URL.Query().Get(":name"))
	}))
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: s.config.TLSConfig,
		},
		Timeout: testhelpers.LongWait,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Get(url + "/hello/world")
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	out, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, "hello, world")
}

func (s *WorkerSuite) TestWaitsForClients(c *tc.C) {
	// Check that the httpserver stays functional until any clients
	// have finished with it.
	s.mux.AddClient()

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

	s.mux.ClientDone()
	select {
	case err := <-waitResult:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("didn't stop after clients were finished")
	}
}

func (s *WorkerSuite) TestExitsWithTardyClients(c *tc.C) {
	// Check that the httpserver shuts down eventually if
	// clients appear to be stuck.
	s.mux.AddClient()

	// Shouldn't take effect until the timeout.
	s.worker.Kill()

	waitResult := make(chan error)
	go func() {
		waitResult <- s.worker.Wait()
	}()

	select {
	case <-waitResult:
		c.Fatalf("didn't wait for timeout")
	case <-time.After(coretesting.ShortWait):
	}

	// Don't call s.mux.ClientDone(), timeout instead.
	s.clock.Advance(1 * time.Minute)
	select {
	case err := <-waitResult:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("didn't stop after timeout")
	}
}

func (s *WorkerSuite) TestMinTLSVersion(c *tc.C) {
	parsed, err := url.Parse(s.worker.URL())
	c.Assert(err, tc.ErrorIsNil)

	tlsConfig := s.config.TLSConfig
	// Specify an unsupported TLS version
	tlsConfig.MaxVersion = tls.VersionSSL30

	conn, err := tls.Dial("tcp", parsed.Host, tlsConfig)
	c.Assert(err, tc.ErrorMatches, ".*tls:.*version.*")
	c.Assert(conn, tc.IsNil)
}

func (s *WorkerSuite) TestGracefulShutdown(c *tc.C) {
	// Worker url comes back as "" when the worker is dying.
	url := s.worker.URL()

	// Simulate having a slow request being handled.
	s.mux.AddClient()

	err := s.mux.AddHandler("GET", "/quick", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	c.Assert(err, tc.ErrorIsNil)

	quickErr := make(chan error)
	request := func() {
		// Make a new client each request so we don't reuse
		// connections.
		client := &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives: true,
				TLSClientConfig:   s.config.TLSConfig,
			},
			Timeout: testhelpers.LongWait,
		}
		defer client.CloseIdleConnections()

		resp, err := client.Get(url + "/quick")
		if err != nil {
			quickErr <- err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			quickErr <- fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			return
		}
		quickErr <- nil
	}

	// Sanity check - the quick one should be quick normally.
	go request()

	select {
	case err := <-quickErr:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for quick request")
	}

	// Stop the server.
	s.worker.Kill()

	// We actually try the quick request more than once, on the off chance
	// that the worker hasn't finished processing the kill signal. Since we have
	// no other way to check, we just try a quick request, and decide that if
	// it doesn't respond quickly, the main loop is waiting for the clients to
	// be done.
	timeout := time.After(coretesting.LongWait)
LOOP:
	for {
		c.Log("try to hit the quick endpoint")

		go request()
		select {
		case err := <-quickErr:
			if errors.Is(err, syscall.ECONNREFUSED) {
				c.Logf("  got a connection refused: server is dead")
				break LOOP
			}
			c.Logf("  got a response: %v", err)

			// Prevent the test from spin locking.
			time.Sleep(time.Millisecond * 10)
		case <-time.After(coretesting.ShortWait):
		case <-timeout:
			c.Fatalf("worker not blocking")
		}
	}

	// The server doesn't die yet - it's kept alive by the slow
	// request.
	workertest.CheckAlive(c, s.worker)

	// Let the slow request complete.  See that the server
	// stops, and the 2nd request completes.
	s.mux.ClientDone()

	workertest.CheckKilled(c, s.worker)
}
