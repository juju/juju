// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/juju/loggo/v2"
	"github.com/juju/pubsub/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/introspection"
	_ "github.com/juju/juju/state"
)

type suite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&suite{})

func (s *suite) TestConfigValidation(c *gc.C) {
	w, err := introspection.NewWorker(introspection.Config{})
	c.Check(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "empty SocketName not valid")
	w, err = introspection.NewWorker(introspection.Config{
		SocketName: "socket",
	})
	c.Check(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "nil PrometheusGatherer not valid")
	w, err = introspection.NewWorker(introspection.Config{
		SocketName:         "socket",
		PrometheusGatherer: newPrometheusGatherer(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(w, gc.Not(gc.IsNil))
}

func (s *suite) TestStartStop(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}

	socketName := path.Join(c.MkDir(), "introspection-test")
	w, err := introspection.NewWorker(introspection.Config{
		SocketName:         socketName,
		PrometheusGatherer: prometheus.NewRegistry(),
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckKill(c, w)
}

type introspectionSuite struct {
	testing.IsolationSuite

	name       string
	worker     worker.Worker
	reporter   introspection.DepEngineReporter
	gatherer   prometheus.Gatherer
	centralHub introspection.StructuredHub
}

var _ = gc.Suite(&introspectionSuite{})

func (s *introspectionSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}
	s.IsolationSuite.SetUpTest(c)
	s.reporter = nil
	s.worker = nil
	s.gatherer = newPrometheusGatherer()
	s.centralHub = pubsub.NewStructuredHub(&pubsub.StructuredHubConfig{Logger: loggo.GetLogger("test.centralhub")})
	s.startWorker(c)
}

func (s *introspectionSuite) startWorker(c *gc.C) {
	s.name = path.Join(c.MkDir(), fmt.Sprintf("introspection-test-%d", os.Getpid()))
	w, err := introspection.NewWorker(introspection.Config{
		SocketName:         s.name,
		DepEngine:          s.reporter,
		PrometheusGatherer: s.gatherer,
		CentralHub:         s.centralHub,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.worker = w
	s.AddCleanup(func(c *gc.C) {
		workertest.CleanKill(c, w)
	})
}

func (s *introspectionSuite) call(c *gc.C, path string) *http.Response {
	client := unixSocketHTTPClient(s.name)
	c.Assert(strings.HasPrefix(path, "/"), jc.IsTrue)
	targetURL, err := url.Parse("http://unix.socket" + path)
	c.Assert(err, jc.ErrorIsNil)

	resp, err := client.Get(targetURL.String())
	c.Assert(err, jc.ErrorIsNil)
	return resp
}

func (s *introspectionSuite) body(c *gc.C, r *http.Response) string {
	response, err := io.ReadAll(r.Body)
	c.Assert(err, jc.ErrorIsNil)
	return string(response)
}

func (s *introspectionSuite) assertBody(c *gc.C, response *http.Response, value string) {
	body := s.body(c, response)
	c.Assert(body, gc.Equals, value+"\n")
}

func (s *introspectionSuite) assertContains(c *gc.C, value, expected string) {
	c.Assert(strings.Contains(value, expected), jc.IsTrue,
		gc.Commentf("missing %q in %v", expected, value))
}

func (s *introspectionSuite) assertBodyContains(c *gc.C, response *http.Response, value string) {
	body := s.body(c, response)
	s.assertContains(c, body, value)
}

func (s *introspectionSuite) TestCmdLine(c *gc.C) {
	response := s.call(c, "/debug/pprof/cmdline")
	s.assertBodyContains(c, response, "/introspection.test")
}

func (s *introspectionSuite) TestGoroutineProfile(c *gc.C) {
	response := s.call(c, "/debug/pprof/goroutine?debug=1")
	body := s.body(c, response)
	c.Check(body, gc.Matches, `(?s)^goroutine profile: total \d+.*`)
}

func (s *introspectionSuite) TestTrace(c *gc.C) {
	response := s.call(c, "/debug/pprof/trace?seconds=1")
	c.Assert(response.Header.Get("Content-Type"), gc.Equals, "application/octet-stream")
}

func (s *introspectionSuite) TestMissingDepEngineReporter(c *gc.C) {
	response := s.call(c, "/depengine")
	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
	s.assertBody(c, response, "missing dependency engine reporter")
}

func (s *introspectionSuite) TestMissingStatePoolReporter(c *gc.C) {
	response := s.call(c, "/statepool")
	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
	s.assertBody(c, response, `"State Pool" introspection not supported`)
}

func (s *introspectionSuite) TestMissingPubSubReporter(c *gc.C) {
	response := s.call(c, "/pubsub")
	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
	s.assertBody(c, response, `"PubSub Report" introspection not supported`)
}

func (s *introspectionSuite) TestMissingMachineLock(c *gc.C) {
	response := s.call(c, "/machinelock")
	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
	s.assertBody(c, response, "missing machine lock reporter")
}

func (s *introspectionSuite) TestStateTrackerReporter(c *gc.C) {
	response := s.call(c, "/debug/pprof/juju/state/tracker?debug=1")
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	s.assertBodyContains(c, response, "juju/state/tracker profile: total")
}

func (s *introspectionSuite) TestEngineReporter(c *gc.C) {
	// We need to make sure the existing worker is shut down
	// so we can connect to the socket.
	workertest.CheckKill(c, s.worker)
	s.reporter = &reporter{
		values: map[string]interface{}{
			"working": true,
		},
	}
	s.startWorker(c)
	response := s.call(c, "/depengine")
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	// TODO: perhaps make the output of the dependency engine YAML parseable.
	// This could be done by having the first line start with a '#'.
	s.assertBody(c, response, `
Dependency Engine Report

working: true`[1:])
}

func (s *introspectionSuite) TestPrometheusMetrics(c *gc.C) {
	response := s.call(c, "/metrics")
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	body := s.body(c, response)
	s.assertContains(c, body, "# HELP tau Tau")
	s.assertContains(c, body, "# TYPE tau counter")
	s.assertContains(c, body, "tau 6.283185")
}

type reporter struct {
	values map[string]interface{}
}

func (r *reporter) Report() map[string]interface{} {
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
			return net.Dial("unix", socketPath)
		},
	}
}
