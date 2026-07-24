// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/flightrecorder"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/introspection"
	"github.com/juju/juju/juju/sockets"
)

type suite struct {
	testhelpers.IsolationSuite
}

func TestSuite(t *testing.T) {
	tc.Run(t, &suite{})
}

func (s *suite) TestConfigValidation(c *tc.C) {
	socketName := path.Join(c.MkDir(), "introspection-test.socket")
	_, err := introspection.NewWorker(introspection.Config{})
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	_, err = introspection.NewWorker(introspection.Config{
		SocketName: socketName,
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	_, err = introspection.NewWorker(introspection.Config{
		SocketName:         socketName,
		PrometheusGatherer: newPrometheusGatherer(),
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	w, err := introspection.NewWorker(introspection.Config{
		SocketName:         socketName,
		PrometheusGatherer: newPrometheusGatherer(),
		FlightRecorder:     flightRecorder{},
	})
	c.Assert(err, tc.ErrorIsNil)

	defer workertest.CleanKill(c, w)
}

func (s *suite) TestStartStop(c *tc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}

	socketName := path.Join(c.MkDir(), "introspection-test.socket")
	w, err := introspection.NewWorker(introspection.Config{
		SocketName:         socketName,
		PrometheusGatherer: prometheus.NewRegistry(),
		FlightRecorder:     flightRecorder{},
	})
	c.Assert(err, tc.ErrorIsNil)
	_ = workertest.CheckKill(c, w)
}

type introspectionSuite struct {
	testhelpers.IsolationSuite

	name              string
	controlSocketPath string
	worker            worker.Worker
	depEngine         introspection.DependencyEngine
	gatherer          prometheus.Gatherer
	flightRecorder    flightrecorder.FlightRecorder
}

func TestIntrospectionSuite(t *testing.T) {
	tc.Run(t, &introspectionSuite{})
}

func (s *introspectionSuite) SetUpTest(c *tc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}
	s.IsolationSuite.SetUpTest(c)
	s.depEngine = nil
	s.worker = nil
	s.controlSocketPath = ""
	s.gatherer = newPrometheusGatherer()
	s.flightRecorder = flightRecorder{}
	s.startWorker(c)
}

func (s *introspectionSuite) startWorker(c *tc.C) {
	s.name = path.Join(c.MkDir(), fmt.Sprintf("introspection-test-%d.socket", os.Getpid()))
	w, err := introspection.NewWorker(introspection.Config{
		SocketName:         s.name,
		ControlSocketPath:  s.controlSocketPath,
		DepEngine:          s.depEngine,
		PrometheusGatherer: s.gatherer,
		FlightRecorder:     s.flightRecorder,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.worker = w
	s.AddCleanup(func(c *tc.C) {
		workertest.CleanKill(c, w)
	})
}

func (s *introspectionSuite) call(c *tc.C, path string) *http.Response {
	return s.request(c, http.MethodGet, path)
}

func (s *introspectionSuite) post(c *tc.C, path string) *http.Response {
	return s.request(c, http.MethodPost, path)
}

func (s *introspectionSuite) request(
	c *tc.C, method, path string,
) *http.Response {
	client := unixSocketHTTPClient(s.name)
	c.Assert(strings.HasPrefix(path, "/"), tc.IsTrue)
	targetURL, err := url.Parse("http://unix.socket" + path)
	c.Assert(err, tc.ErrorIsNil)

	req, err := http.NewRequest(method, targetURL.String(), nil)
	c.Assert(err, tc.ErrorIsNil)
	resp, err := client.Do(req)
	c.Assert(err, tc.ErrorIsNil)
	return resp
}

func (s *introspectionSuite) body(c *tc.C, r *http.Response) string {
	response, err := io.ReadAll(r.Body)
	c.Assert(err, tc.ErrorIsNil)
	return string(response)
}

func (s *introspectionSuite) assertBody(c *tc.C, response *http.Response, value string) {
	body := s.body(c, response)
	c.Assert(body, tc.Equals, value+"\n")
}

func (s *introspectionSuite) assertContains(c *tc.C, value, expected string) {
	c.Assert(strings.Contains(value, expected), tc.IsTrue,
		tc.Commentf("missing %q in %v", expected, value))
}

func (s *introspectionSuite) assertBodyContains(c *tc.C, response *http.Response, value string) {
	body := s.body(c, response)
	s.assertContains(c, body, value)
}

func (s *introspectionSuite) TestCmdLine(c *tc.C) {
	response := s.call(c, "/debug/pprof/cmdline")
	defer response.Body.Close()
	s.assertBodyContains(c, response, "/introspection.test")
}

func (s *introspectionSuite) TestGoroutineProfile(c *tc.C) {
	response := s.call(c, "/debug/pprof/goroutine?debug=1")
	defer response.Body.Close()
	body := s.body(c, response)
	c.Check(body, tc.Matches, `(?s)^goroutine profile: total \d+.*`)
}

func (s *introspectionSuite) TestTrace(c *tc.C) {
	response := s.call(c, "/debug/pprof/trace?seconds=1")
	defer response.Body.Close()
	c.Assert(response.Header.Get("Content-Type"), tc.Equals, "application/octet-stream")
}

func (s *introspectionSuite) TestMissingDepEngineReporter(c *tc.C) {
	response := s.call(c, "/depengine")
	defer response.Body.Close()
	c.Assert(response.StatusCode, tc.Equals, http.StatusNotFound)
	s.assertBody(c, response, "missing dependency engine reporter")
}

func (s *introspectionSuite) TestMissingMachineLock(c *tc.C) {
	response := s.call(c, "/machinelock")
	defer response.Body.Close()
	c.Assert(response.StatusCode, tc.Equals, http.StatusNotFound)
	s.assertBody(c, response, "missing machine lock reporter")
}

func (s *introspectionSuite) TestEngineReporter(c *tc.C) {
	// We need to make sure the existing worker is shut down
	// so we can connect to the socket.
	workertest.CleanKill(c, s.worker)
	s.depEngine = &depEngine{
		values: map[string]any{
			"working": true,
		},
	}
	s.startWorker(c)
	response := s.call(c, "/depengine")
	defer response.Body.Close()
	c.Assert(response.StatusCode, tc.Equals, http.StatusOK)
	// TODO: perhaps make the output of the dependency engine YAML parseable.
	// This could be done by having the first line start with a '#'.
	s.assertBody(c, response, `
Dependency Engine Report

working: true`[1:])
}

func (s *introspectionSuite) TestPrometheusMetrics(c *tc.C) {
	response := s.call(c, "/metrics")
	defer response.Body.Close()
	c.Assert(response.StatusCode, tc.Equals, http.StatusOK)
	body := s.body(c, response)
	s.assertContains(c, body, "# HELP tau Tau")
	s.assertContains(c, body, "# TYPE tau counter")
	s.assertContains(c, body, "tau 6.283185")
}

func (s *introspectionSuite) TestObjectStoreReadRepairNotConfigured(c *tc.C) {
	response := s.post(c, "/objectstore/read-repair")
	defer response.Body.Close()
	c.Assert(response.StatusCode, tc.Equals, http.StatusNotFound)
}

func (s *introspectionSuite) TestObjectStoreReadRepairInvalidMethod(c *tc.C) {
	workertest.CleanKill(c, s.worker)
	s.controlSocketPath = path.Join(c.MkDir(), "control.socket")
	s.startWorker(c)

	response := s.call(c, "/objectstore/read-repair")
	defer response.Body.Close()
	c.Assert(response.StatusCode, tc.Equals, http.StatusMethodNotAllowed)
	s.assertBody(c, response, "method not allowed")
}

func (s *introspectionSuite) TestObjectStoreReadRepair(c *tc.C) {
	socketPath := s.startReadRepairControlSocketServer(c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, http.MethodPost)
		c.Check(r.URL.Path, tc.Equals, "/objectstore/read-repair")
		c.Check(r.Header.Get("Content-Type"), tc.Equals, "application/json")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))

	workertest.CleanKill(c, s.worker)
	s.controlSocketPath = socketPath
	s.startWorker(c)

	response := s.post(c, "/objectstore/read-repair")
	defer response.Body.Close()
	c.Assert(response.StatusCode, tc.Equals, http.StatusOK)
	c.Assert(response.Header.Get("Content-Type"), tc.Equals, "application/json")
	c.Assert(s.body(c, response), tc.Equals, `{"message":"ok"}`)
}

func (s *introspectionSuite) TestObjectStoreReadRepairProxyError(c *tc.C) {
	workertest.CleanKill(c, s.worker)
	s.controlSocketPath = path.Join(c.MkDir(), "control.socket")
	s.startWorker(c)

	response := s.post(c, "/objectstore/read-repair")
	defer response.Body.Close()
	c.Assert(response.StatusCode, tc.Equals, http.StatusInternalServerError)
	s.assertBody(c, response, "requesting read-repair failed")
}

func (s *introspectionSuite) TestObjectStoreReadRepairPassesBackendConflict(c *tc.C) {
	socketPath := s.startReadRepairControlSocketServer(c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, http.MethodPost)
		c.Check(r.URL.Path, tc.Equals, "/objectstore/read-repair")
		c.Check(r.Header.Get("Content-Type"), tc.Equals, "application/json")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"incomplete"}`))
	}))

	workertest.CleanKill(c, s.worker)
	s.controlSocketPath = socketPath
	s.startWorker(c)

	response := s.post(c, "/objectstore/read-repair")
	defer response.Body.Close()
	c.Assert(response.StatusCode, tc.Equals, http.StatusConflict)
	c.Assert(response.Header.Get("Content-Type"), tc.Equals, "application/json")
	c.Assert(s.body(c, response), tc.Equals, `{"error":"incomplete"}`)
}

func (s *introspectionSuite) startReadRepairControlSocketServer(
	c *tc.C, handler http.Handler,
) string {
	socketPath := path.Join(c.MkDir(), "control.socket")
	listener, err := sockets.Listen(sockets.Socket{
		Network: "unix",
		Address: socketPath,
	})
	c.Assert(err, tc.ErrorIsNil)

	done := make(chan struct{})
	server := &http.Server{
		Handler: handler,
	}
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()
	c.Cleanup(func() {
		_ = server.Close()
		_ = listener.Close()
		<-done
	})
	return socketPath
}

type depEngine struct {
	values map[string]any
}

func (r *depEngine) Report(ctx context.Context) map[string]any {
	return r.values
}

func newPrometheusGatherer() prometheus.Gatherer {
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "tau", Help: "Tau."})
	counter.Add(6.283185)
	r := prometheus.NewPedanticRegistry()
	r.MustRegister(counter)
	return r
}

func unixSocketHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: unixSocketHTTPTransport(socketPath),
		Timeout:   15 * time.Second,
	}
}

func unixSocketHTTPTransport(socketPath string) *http.Transport {
	return &http.Transport{
		Dial: func(proto, addr string) (net.Conn, error) {
			return sockets.Dialer(sockets.Socket{
				Network: "unix",
				Address: socketPath,
			})
		},
	}
}

type flightRecorder struct {
	flightrecorder.FlightRecorder
}
