/// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"regexp"
	"runtime"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	// Bring in the state package for the tracker profile.
	"github.com/juju/juju/core/presence"
	_ "github.com/juju/juju/state"
	"github.com/juju/juju/worker/introspection"
)

type suite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&suite{})

func (s *suite) TestConfigValidation(c *gc.C) {
	w, err := introspection.NewWorker(introspection.Config{})
	c.Check(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "empty SocketName not valid")
}

func (s *suite) TestStartStop(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}

	w, err := introspection.NewWorker(introspection.Config{
		SocketName:         "introspection-test",
		PrometheusGatherer: prometheus.NewRegistry(),
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckKill(c, w)
}

type introspectionSuite struct {
	testing.IsolationSuite

	name     string
	worker   worker.Worker
	reporter introspection.DepEngineReporter
	gatherer prometheus.Gatherer
	recorder presence.Recorder
}

var _ = gc.Suite(&introspectionSuite{})

func (s *introspectionSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}
	s.IsolationSuite.SetUpTest(c)
	s.reporter = nil
	s.worker = nil
	s.recorder = nil
	s.gatherer = newPrometheusGatherer()
	s.startWorker(c)
}

func (s *introspectionSuite) startWorker(c *gc.C) {
	s.name = fmt.Sprintf("introspection-test-%d", os.Getpid())
	w, err := introspection.NewWorker(introspection.Config{
		SocketName:         s.name,
		DepEngine:          s.reporter,
		PrometheusGatherer: s.gatherer,
		Presence:           s.recorder,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.worker = w
	s.AddCleanup(func(c *gc.C) {
		workertest.CheckKill(c, w)
	})
}

func (s *introspectionSuite) call(c *gc.C, url string) []byte {
	path := "@" + s.name
	conn, err := net.Dial("unix", path)
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "GET %s HTTP/1.0\r\n\r\n", url)
	c.Assert(err, jc.ErrorIsNil)

	buf, err := ioutil.ReadAll(conn)
	c.Assert(err, jc.ErrorIsNil)
	return buf
}

func (s *introspectionSuite) TestCmdLine(c *gc.C) {
	buf := s.call(c, "/debug/pprof/cmdline")
	c.Assert(buf, gc.NotNil)
	matches(c, buf, ".*/introspection.test")
}

func (s *introspectionSuite) TestGoroutineProfile(c *gc.C) {
	buf := s.call(c, "/debug/pprof/goroutine?debug=1")
	c.Assert(buf, gc.NotNil)
	matches(c, buf, `^goroutine profile: total \d+`)
}

func (s *introspectionSuite) TestTrace(c *gc.C) {
	buf := s.call(c, "/debug/pprof/trace?seconds=1")
	c.Assert(buf, gc.NotNil)
	matches(c, buf, `^Content-Type: application/octet-stream*`)
}

func (s *introspectionSuite) TestMissingDepEngineReporter(c *gc.C) {
	buf := s.call(c, "/depengine")
	matches(c, buf, "404 Not Found")
	matches(c, buf, "missing dependency engine reporter")
}

func (s *introspectionSuite) TestMissingStatePoolReporter(c *gc.C) {
	buf := s.call(c, "/statepool")
	matches(c, buf, "404 Not Found")
	matches(c, buf, "State Pool Report: missing reporter")
}

func (s *introspectionSuite) TestMissingPubSubReporter(c *gc.C) {
	buf := s.call(c, "/pubsub")
	matches(c, buf, "404 Not Found")
	matches(c, buf, "PubSub Report: missing reporter")
}

func (s *introspectionSuite) TestMissingMachineLock(c *gc.C) {
	buf := s.call(c, "/machinelock/")
	matches(c, buf, "404 Not Found")
	matches(c, buf, "missing machine lock reporter")
}

func (s *introspectionSuite) TestStateTrackerReporter(c *gc.C) {
	buf := s.call(c, "/debug/pprof/juju/state/tracker?debug=1")
	matches(c, buf, "200 OK")
	matches(c, buf, "juju/state/tracker profile: total")
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
	buf := s.call(c, "/depengine")

	matches(c, buf, "200 OK")
	matches(c, buf, "working: true")
}

func (s *introspectionSuite) TestMissingPresenceReporter(c *gc.C) {
	buf := s.call(c, "/presence/")
	matches(c, buf, "404 Not Found")
	matches(c, buf, "page not found")
}

func (s *introspectionSuite) TestDisabledPresenceReporter(c *gc.C) {
	// We need to make sure the existing worker is shut down
	// so we can connect to the socket.
	workertest.CheckKill(c, s.worker)
	s.recorder = presence.New(testclock.NewClock(time.Now()))
	s.startWorker(c)

	buf := s.call(c, "/presence/")
	matches(c, buf, "404 Not Found")
	matches(c, buf, "agent is not an apiserver")
}

func (s *introspectionSuite) TestEnabledPresenceReporter(c *gc.C) {
	// We need to make sure the existing worker is shut down
	// so we can connect to the socket.
	workertest.CheckKill(c, s.worker)
	s.recorder = presence.New(testclock.NewClock(time.Now()))
	s.recorder.Enable()
	s.recorder.Connect("server", "model-uuid", "agent-1", 42, false, "")
	s.startWorker(c)

	buf := s.call(c, "/presence/")
	matches(c, buf, "200 OK")
	matches(c, buf, "AGENT    SERVER  CONN ID  STATUS")
	matches(c, buf, "agent-1  server  42       alive")
}

func (s *introspectionSuite) TestPrometheusMetrics(c *gc.C) {
	buf := s.call(c, "/metrics/")
	c.Assert(buf, gc.NotNil)
	matches(c, buf, "# HELP tau Tau")
	matches(c, buf, "# TYPE tau counter")
	matches(c, buf, "tau 6.283185")
}

// matches fails if regex is not found in the contents of b.
// b is expected to be the response from the pprof http server, and will
// contain some HTTP preamble that should be ignored.
func matches(c *gc.C, b []byte, regex string) {
	re, err := regexp.Compile(regex)
	c.Assert(err, jc.ErrorIsNil)
	r := bytes.NewReader(b)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if re.MatchString(sc.Text()) {
			return
		}
	}
	c.Fatalf("%q did not match regex %q", string(b), regex)
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
