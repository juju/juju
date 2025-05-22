// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	dqlitetesting "github.com/juju/juju/internal/database/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pubsub/apiserver"
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
	hub                  *pubsub.StructuredHub
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
	s.hub = pubsub.NewStructuredHub(nil)
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
		Hub:                  s.hub,
		APIPort:              0,
		APIPortOpenDelay:     0,
		ControllerAPIPort:    0,
		Logger:               loggertesting.WrapCheckLog(c),
	}
}

type WorkerValidationSuite struct {
	workerFixture
}

func TestWorkerValidationSuite(t *stdtesting.T) {
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

func TestWorkerSuite(t *stdtesting.T) {
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
	// Normal exit, no debug file.
	_, err := os.Stat(filepath.Join(s.logDir, "apiserver-debug.log"))
	c.Assert(err, tc.Satisfies, os.IsNotExist)
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
	// There should be a log file with goroutines.
	data, err := os.ReadFile(filepath.Join(s.logDir, "apiserver-debug.log"))
	c.Assert(err, tc.ErrorIsNil)
	lines := strings.Split(string(data), "\n")
	c.Assert(len(lines), tc.GreaterThan, 1)
	c.Assert(lines[1], tc.Matches, "goroutine profile:.*")
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

func (s *WorkerSuite) TestHeldListener(c *tc.C) {
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
				TLSClientConfig: s.config.TLSConfig,
			},
			Timeout: testhelpers.LongWait,
		}
		defer client.CloseIdleConnections()
		_, err := client.Get(url + "/quick")
		quickErr <- err
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

	// A very small sleep will allow the kill to be more likely to be processed
	// by the running loop.
	time.Sleep(10 * time.Millisecond)

	// We actually try the quick request more than once, on the off chance
	// that the worker hasn't finished processing the kill signal. Since we have
	// no other way to check, we just try a quick request, and decide that if
	// it doesn't respond quickly, the main loop is waithing for the clients to
	// be done.
	quickBlocked := false
	timeout := time.After(coretesting.LongWait)
	for !quickBlocked {
		c.Log("try to hit the quick endpoint")
		go request()
		select {
		case <-quickErr:
			c.Log("  got a response")
		case <-time.After(coretesting.ShortWait):
			quickBlocked = true
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

	select {
	case <-quickErr:
		// There is a race in the queueing of the request. It is possible that
		// the timer will fire a short wait before an unheld request gets to the
		// phase where it would return nil. However this is only under significant
		// load, and it isn't easy to synchronise. This is why we don't actually
		// check the error.
		// It doesn't really matter what the error is.
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for 2nd quick request")
	}
	workertest.CheckKilled(c, s.worker)
}

type WorkerControllerPortSuite struct {
	workerFixture
}

func TestWorkerControllerPortSuite(t *stdtesting.T) {
	tc.Run(t, &WorkerControllerPortSuite{})
}
func (s *WorkerControllerPortSuite) newWorker(c *tc.C) *httpserver.Worker {
	worker, err := httpserver.NewWorker(s.config)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) {
		workertest.DirtyKill(c, worker)
	})
	return worker
}

func (s *WorkerControllerPortSuite) TestDualPortListenerWithDelay(c *tc.C) {
	err := s.mux.AddHandler("GET", "/quick", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	c.Assert(err, tc.ErrorIsNil)

	request := func(url string) error {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: s.config.TLSConfig,
			},
			Timeout: testhelpers.LongWait,
		}
		defer client.CloseIdleConnections()
		_, err := client.Get(url + "/quick")
		return err
	}

	// Make a worker with a controller API port.
	port := dqlitetesting.FindTCPPort(c)
	controllerPort := dqlitetesting.FindTCPPort(c)
	s.config.APIPort = port
	s.config.ControllerAPIPort = controllerPort
	s.config.APIPortOpenDelay = 10 * time.Second

	worker := s.newWorker(c)

	// The worker reports its URL as the controller port.
	controllerURL := worker.URL()
	parsed, err := url.Parse(controllerURL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(parsed.Port(), tc.Equals, fmt.Sprint(controllerPort))

	reportPorts := map[string]interface{}{
		"controller": fmt.Sprintf("[::]:%d", s.config.ControllerAPIPort),
		"status":     "waiting for signal to open agent port",
	}
	report := map[string]interface{}{
		"api-port":            s.config.APIPort,
		"api-port-open-delay": s.config.APIPortOpenDelay,
		"controller-api-port": s.config.ControllerAPIPort,
		"status":              "running",
		"ports":               reportPorts,
	}
	c.Check(worker.Report(), tc.DeepEquals, report)

	// Requests on that port work.
	c.Assert(request(controllerURL), tc.ErrorIsNil)

	// Requests on the regular API port fail to connect.
	parsed.Host = net.JoinHostPort(parsed.Hostname(), fmt.Sprint(port))
	normalURL := parsed.String()
	c.Assert(request(normalURL), tc.ErrorMatches, `.*: connection refused$`)

	// Getting a connection from someone else doesn't unblock.
	handled, err := s.hub.Publish(apiserver.ConnectTopic, apiserver.APIConnection{
		AgentTag: "machine-13",
		Origin:   s.agentName,
	})
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("the handler should have exited early and not be waiting")
	}

	// Send API details on the hub - still no luck connecting on the
	// non-controller port.
	_, err = s.hub.Publish(apiserver.ConnectTopic, apiserver.APIConnection{
		AgentTag: s.agentName,
		Origin:   s.agentName,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.clock.WaitAdvance(5*time.Second, coretesting.LongWait, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(request(controllerURL), tc.ErrorIsNil)
	c.Assert(request(normalURL), tc.ErrorMatches, `.*: connection refused$`)

	reportPorts["status"] = "waiting prior to opening agent port"
	c.Check(worker.Report(), tc.DeepEquals, report)

	// After the required delay the port eventually opens.
	err = s.clock.WaitAdvance(5*time.Second, coretesting.LongWait, 1)
	c.Assert(err, tc.ErrorIsNil)

	// The reported url changes to the regular port.
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if worker.URL() == normalURL {
			break
		}
	}
	c.Assert(worker.URL(), tc.Equals, normalURL)

	// Requests on both ports work.
	c.Assert(request(controllerURL), tc.ErrorIsNil)
	c.Assert(request(normalURL), tc.ErrorIsNil)

	delete(reportPorts, "status")
	reportPorts["agent"] = fmt.Sprintf("[::]:%d", s.config.APIPort)
	c.Check(worker.Report(), tc.DeepEquals, report)
}

func (s *WorkerControllerPortSuite) TestDualPortListenerWithDelayShutdown(c *tc.C) {
	err := s.mux.AddHandler("GET", "/quick", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	c.Assert(err, tc.ErrorIsNil)

	request := func(url string) error {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: s.config.TLSConfig,
			},
			Timeout: testhelpers.LongWait,
		}
		defer client.CloseIdleConnections()
		_, err := client.Get(url + "/quick")
		return err
	}
	// Make a worker with a controller API port.
	port := dqlitetesting.FindTCPPort(c)
	controllerPort := dqlitetesting.FindTCPPort(c)
	s.config.APIPort = port
	s.config.ControllerAPIPort = controllerPort
	s.config.APIPortOpenDelay = 10 * time.Second

	worker := s.newWorker(c)
	controllerURL := worker.URL()
	parsed, err := url.Parse(controllerURL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(parsed.Port(), tc.Equals, fmt.Sprint(controllerPort))
	// Requests to controllerURL are successful, but normal requests are denied
	c.Assert(request(controllerURL), tc.ErrorIsNil)
	parsed.Host = net.JoinHostPort(parsed.Hostname(), fmt.Sprint(port))
	normalURL := parsed.String()
	c.Assert(request(normalURL), tc.ErrorMatches, `.*: connection refused$`)
	// Send API details on the hub - still no luck connecting on the
	// non-controller port.
	_, err = s.hub.Publish(apiserver.ConnectTopic, apiserver.APIConnection{
		AgentTag: s.agentName,
		Origin:   s.agentName,
	})
	c.Assert(err, tc.ErrorIsNil)
	// We exit cleanly even if we never tick the clock forward
	workertest.CleanKill(c, worker)
}
