// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/pubsub/apiserver"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/httpserver"
)

type workerFixture struct {
	testing.IsolationSuite
	prometheusRegisterer stubPrometheusRegisterer
	mux                  *apiserverhttp.Mux
	clock                *testing.Clock
	hub                  *pubsub.StructuredHub
	config               httpserver.Config
	stub                 testing.Stub
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	certPool, err := api.CreateCertPool(coretesting.CACert)
	c.Assert(err, jc.ErrorIsNil)
	tlsConfig := api.NewTLSConfig(certPool)
	tlsConfig.ServerName = "juju-apiserver"
	tlsConfig.Certificates = []tls.Certificate{*coretesting.ServerTLSCert}
	s.prometheusRegisterer = stubPrometheusRegisterer{}
	s.mux = apiserverhttp.NewMux()
	s.clock = testing.NewClock(time.Now())
	s.hub = pubsub.NewStructuredHub(nil)

	s.config = httpserver.Config{
		Clock:                s.clock,
		TLSConfig:            tlsConfig,
		Mux:                  s.mux,
		PrometheusRegisterer: &s.prometheusRegisterer,
		Hub:                  s.hub,
		APIPort:              0,
		APIPortOpenDelay:     0,
		ControllerAPIPort:    0,
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
		func(cfg *httpserver.Config) { cfg.TLSConfig = nil },
		"nil TLSConfig not valid",
	}, {
		func(cfg *httpserver.Config) { cfg.Mux = nil },
		"nil Mux not valid",
	}, {
		func(cfg *httpserver.Config) { cfg.PrometheusRegisterer = nil },
		"nil PrometheusRegisterer not valid",
	}, {
		func(cfg *httpserver.Config) {
			cfg.AutocertHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
		},
		"AutocertListener must not be nil if AutocertHandler is not nil",
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

func (s *WorkerSuite) TestHeldListener(c *gc.C) {
	// Worker url comes back as "" when the worker is dying.
	url := s.worker.URL()

	// Simulate having a slow request being handled.
	s.mux.AddClient()

	err := s.mux.AddHandler("GET", "/quick", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	c.Assert(err, jc.ErrorIsNil)

	quickErr := make(chan error)
	request := func() {
		// Make a new client each request so we don't reuse
		// connections.
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: s.config.TLSConfig,
			},
		}
		_, err := client.Get(url + "/quick")
		quickErr <- err
	}

	// Sanity check - the quick one should be quick normally.
	go request()

	select {
	case err := <-quickErr:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for quick request")
	}

	// Stop the server.
	s.worker.Kill()

	// Eventually quick requests get blocked by the held listener.
	var quickBlocked bool
attempts:
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		go request()
		select {
		case <-quickErr:
		case <-time.After(coretesting.ShortWait):
			quickBlocked = true
			break attempts
		}
	}
	c.Assert(quickBlocked, gc.Equals, true)

	// The server doesn't die yet - it's kept alive by the slow
	// request.
	workertest.CheckAlive(c, s.worker)

	// Let the slow request complete.  See that the server
	// stops, and the 2nd request completes.
	s.mux.ClientDone()

	select {
	case <-quickErr:
		// It doesn't really matter what the error is, and
		// occasionally we don't get any error.
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for 2nd quick request")
	}
	workertest.CheckKilled(c, s.worker)
}

type WorkerControllerPortSuite struct {
	workerFixture
}

var _ = gc.Suite(&WorkerControllerPortSuite{})

func (s *WorkerControllerPortSuite) newWorker(c *gc.C) *httpserver.Worker {
	worker, err := httpserver.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
	})
	return worker
}

func (s *WorkerControllerPortSuite) TestDualPortListenerWithDelay(c *gc.C) {
	err := s.mux.AddHandler("GET", "/quick", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	c.Assert(err, jc.ErrorIsNil)

	request := func(url string) error {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: s.config.TLSConfig,
			},
		}
		_, err := client.Get(url + "/quick")
		return err
	}

	// Make a worker with a controller API port.
	port := testing.FindTCPPort()
	controllerPort := testing.FindTCPPort()
	s.config.APIPort = port
	s.config.ControllerAPIPort = controllerPort
	s.config.APIPortOpenDelay = 10 * time.Second

	worker := s.newWorker(c)

	// The worker reports its URL as the controller port.
	controllerURL := worker.URL()
	parsed, err := url.Parse(controllerURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(parsed.Port(), gc.Equals, fmt.Sprint(controllerPort))

	// Requests on that port work.
	c.Assert(request(controllerURL), jc.ErrorIsNil)

	// Requests on the regular API port fail to connect.
	parsed.Host = net.JoinHostPort(parsed.Hostname(), fmt.Sprint(port))
	normalURL := parsed.String()
	c.Assert(request(normalURL), gc.ErrorMatches, `.*: connection refused$`)

	// Send API details on the hub - still no luck connecting on the
	// non-controller port.
	_, err = s.hub.Publish(apiserver.DetailsTopic, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.clock.WaitAdvance(5*time.Second, coretesting.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(request(controllerURL), jc.ErrorIsNil)
	c.Assert(request(normalURL), gc.ErrorMatches, `.*: connection refused$`)

	// After the required delay the port eventually opens.
	err = s.clock.WaitAdvance(5*time.Second, coretesting.LongWait, 1)

	// The reported url changes to the regular port.
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if worker.URL() == normalURL {
			break
		}
	}
	c.Assert(worker.URL(), gc.Equals, normalURL)

	// Requests on both ports work.
	c.Assert(request(controllerURL), jc.ErrorIsNil)
	c.Assert(request(normalURL), jc.ErrorIsNil)
}

type WorkerAutocertSuite struct {
	workerFixture
	stub   testing.Stub
	worker *httpserver.Worker
	url    string
}

var _ = gc.Suite(&WorkerAutocertSuite{})

func (s *WorkerAutocertSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)
	s.config.AutocertHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.stub.AddCall("AutocertHandler")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("yay\n"))
	})
	listener, err := net.Listen("tcp", ":0")
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { listener.Close() })
	s.config.AutocertListener = listener
	s.url = fmt.Sprintf("http://%s/whatever/", listener.Addr())
	worker, err := httpserver.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
	})
	s.worker = worker
}

func (s *WorkerAutocertSuite) TestAutocertHandler(c *gc.C) {
	client := &http.Client{}
	response, err := client.Get(s.url)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	content, err := ioutil.ReadAll(response.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "yay\n")

	workertest.CleanKill(c, s.worker)

	_, err = client.Get(s.url)
	c.Assert(err, gc.ErrorMatches, ".*connection refused$")
}
