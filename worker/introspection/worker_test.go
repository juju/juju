// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	// Bring in the state package for the tracker profile.
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/pubsub/agent"
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
	hub      *pubsub.SimpleHub
	clock    *testclock.Clock
	leases   *fakeLeases
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
	s.hub = pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{Logger: loggo.GetLogger("test.hub")})
	s.clock = testclock.NewClock(time.Now())
	s.leases = &fakeLeases{}
	s.startWorker(c)
}

func (s *introspectionSuite) startWorker(c *gc.C) {
	s.name = fmt.Sprintf("introspection-test-%d", os.Getpid())
	w, err := introspection.NewWorker(introspection.Config{
		SocketName:         s.name,
		DepEngine:          s.reporter,
		PrometheusGatherer: s.gatherer,
		Presence:           s.recorder,
		Clock:              s.clock,
		Hub:                s.hub,
		Leases:             s.leases,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.worker = w
	s.AddCleanup(func(c *gc.C) {
		workertest.CleanKill(c, w)
	})
}

func (s *introspectionSuite) call(c *gc.C, path string) *http.Response {
	client := unixSocketHTTPClient("@" + s.name)
	c.Assert(strings.HasPrefix(path, "/"), jc.IsTrue)
	targetURL, err := url.Parse("http://unix.socket" + path)
	c.Assert(err, jc.ErrorIsNil)

	resp, err := client.Get(targetURL.String())
	c.Assert(err, jc.ErrorIsNil)
	return resp
}

func (s *introspectionSuite) post(c *gc.C, path string, values url.Values) *http.Response {
	client := unixSocketHTTPClient("@" + s.name)
	c.Assert(strings.HasPrefix(path, "/"), jc.IsTrue)
	targetURL, err := url.Parse("http://unix.socket" + path)
	c.Assert(err, jc.ErrorIsNil)

	resp, err := client.PostForm(targetURL.String(), values)
	c.Assert(err, jc.ErrorIsNil)
	return resp
}

func (s *introspectionSuite) body(c *gc.C, r *http.Response) string {
	response, err := ioutil.ReadAll(r.Body)
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
	s.assertBody(c, response, "State Pool Report: missing reporter")
}

func (s *introspectionSuite) TestMissingPubSubReporter(c *gc.C) {
	response := s.call(c, "/pubsub")
	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
	s.assertBody(c, response, "PubSub Report: missing reporter")
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

func (s *introspectionSuite) TestMissingPresenceReporter(c *gc.C) {
	response := s.call(c, "/presence")
	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
	s.assertBody(c, response, "404 page not found")
}

func (s *introspectionSuite) TestDisabledPresenceReporter(c *gc.C) {
	// We need to make sure the existing worker is shut down
	// so we can connect to the socket.
	workertest.CheckKill(c, s.worker)
	s.recorder = presence.New(testclock.NewClock(time.Now()))
	s.startWorker(c)

	response := s.call(c, "/presence")
	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
	s.assertBody(c, response, "agent is not an apiserver")
}

func (s *introspectionSuite) TestEnabledPresenceReporter(c *gc.C) {
	// We need to make sure the existing worker is shut down
	// so we can connect to the socket.
	workertest.CheckKill(c, s.worker)
	s.recorder = presence.New(testclock.NewClock(time.Now()))
	s.recorder.Enable()
	s.recorder.Connect("server", "model-uuid", "agent-1", 42, false, "")
	s.startWorker(c)

	response := s.call(c, "/presence")
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	s.assertBody(c, response, `
[model-uuid]

AGENT    SERVER  CONN ID  STATUS
agent-1  server  42       alive
`[1:])
}

func (s *introspectionSuite) TestPrometheusMetrics(c *gc.C) {
	response := s.call(c, "/metrics")
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	body := s.body(c, response)
	s.assertContains(c, body, "# HELP tau Tau")
	s.assertContains(c, body, "# TYPE tau counter")
	s.assertContains(c, body, "tau 6.283185")
}

func (s *introspectionSuite) TestUnitMissingAction(c *gc.C) {
	response := s.call(c, "/units")
	c.Assert(response.StatusCode, gc.Equals, http.StatusBadRequest)
	s.assertBody(c, response, "missing action")
}

func (s *introspectionSuite) TestUnitUnknownAction(c *gc.C) {
	response := s.post(c, "/units", url.Values{"action": {"foo"}})
	c.Assert(response.StatusCode, gc.Equals, http.StatusBadRequest)
	s.assertBody(c, response, `unknown action: "foo"`)
}

func (s *introspectionSuite) TestUnitStartWithGet(c *gc.C) {
	response := s.call(c, "/units?action=start")
	c.Assert(response.StatusCode, gc.Equals, http.StatusMethodNotAllowed)
	s.assertBody(c, response, "start requires a POST request")
}

func (s *introspectionSuite) TestUnitStartMissingUnits(c *gc.C) {
	response := s.post(c, "/units", url.Values{"action": {"start"}})
	c.Assert(response.StatusCode, gc.Equals, http.StatusBadRequest)
	s.assertBody(c, response, "missing unit")
}

func (s *introspectionSuite) TestUnitStartUnits(c *gc.C) {
	unsub := s.hub.Subscribe(agent.StartUnitTopic, func(topic string, data interface{}) {
		_, ok := data.(agent.Units)
		if !ok {
			c.Fatalf("bad data type: %T", data)
			return
		}
		s.hub.Publish(agent.StartUnitResponseTopic, agent.StartStopResponse{
			"one": "started",
			"two": "not found",
		})
	})
	defer unsub()

	response := s.post(c, "/units", url.Values{"action": {"start"}, "unit": {"one", "two"}})
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	s.assertBody(c, response, "one: started\ntwo: not found")
}

func (s *introspectionSuite) TestUnitStopWithGet(c *gc.C) {
	response := s.call(c, "/units?action=stop")
	c.Assert(response.StatusCode, gc.Equals, http.StatusMethodNotAllowed)
	s.assertBody(c, response, "stop requires a POST request")
}

func (s *introspectionSuite) TestUnitStopMissingUnits(c *gc.C) {
	response := s.post(c, "/units", url.Values{"action": {"stop"}})
	c.Assert(response.StatusCode, gc.Equals, http.StatusBadRequest)
	s.assertBody(c, response, "missing unit")
}

func (s *introspectionSuite) TestUnitStopUnits(c *gc.C) {
	unsub := s.hub.Subscribe(agent.StopUnitTopic, func(topic string, data interface{}) {
		_, ok := data.(agent.Units)
		if !ok {
			c.Fatalf("bad data type: %T", data)
			return
		}
		s.hub.Publish(agent.StopUnitResponseTopic, agent.StartStopResponse{
			"one": "stopped",
			"two": "not found",
		})
	})
	defer unsub()

	response := s.post(c, "/units", url.Values{"action": {"stop"}, "unit": {"one", "two"}})
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	s.assertBody(c, response, "one: stopped\ntwo: not found")
}

func (s *introspectionSuite) TestUnitStatus(c *gc.C) {
	unsub := s.hub.Subscribe(agent.UnitStatusTopic, func(string, interface{}) {
		s.hub.Publish(agent.UnitStatusResponseTopic, agent.Status{
			"one": "running",
			"two": "stopped",
		})
	})
	defer unsub()

	response := s.call(c, "/units?action=status")
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	s.assertBody(c, response, `
one: running
two: stopped`[1:])
}

func (s *introspectionSuite) TestUnitStatusTimeout(c *gc.C) {
	unsub := s.hub.Subscribe(agent.UnitStatusTopic, func(string, interface{}) {
		s.clock.Advance(10 * time.Second)
	})
	defer unsub()

	response := s.call(c, "/units?action=status")
	c.Assert(response.StatusCode, gc.Equals, http.StatusInternalServerError)
	s.assertBody(c, response, "response timed out")
}

func (s *introspectionSuite) TestLeasesErr(c *gc.C) {
	s.leases.err = errors.New("boom")
	response := s.call(c, "/leases")
	c.Assert(response.StatusCode, gc.Equals, http.StatusInternalServerError)
	s.assertBody(c, response, "error: boom")
}

func (s *introspectionSuite) TestLeasesNewerVersion(c *gc.C) {
	s.leases.data = &raftlease.Snapshot{
		Version: 42,
	}
	response := s.call(c, "/leases")
	c.Assert(response.StatusCode, gc.Equals, http.StatusInternalServerError)
	s.assertBody(c, response, "only understand how to show version 1 snapshots")
}

func (s *introspectionSuite) TestLeasesDataNoFilter(c *gc.C) {
	s.setLeaseData()
	response := s.call(c, "/leases")
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	s.assertBody(c, response, `
controller-leases:
  other-uuid:
    holder: controller-1
    lease-acquired: 10s ago
    lease-expires: 50s
  some-uuid:
    holder: controller-0
    lease-acquired: 10s ago
    lease-expires: 50s
model-leases:
  other-uuid:
    keystone:
      holder: keystone/42
      lease-acquired: 10s ago
      lease-expires: 50s
    mysql:
      holder: mysql/1
      lease-acquired: 10s ago
      lease-expires: 50s
  some-uuid:
    mysql:
      holder: mysql/0
      lease-acquired: 10s ago
      lease-expires: 50s
    wordpress:
      holder: wordpress/1
      lease-acquired: 10s ago
      lease-expires: 50s`[1:])
}

func (s *introspectionSuite) TestFilterModelUUID(c *gc.C) {
	s.setLeaseData()
	response := s.call(c, "/leases?model=some")
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	s.assertBody(c, response, `
controller-leases:
  some-uuid:
    holder: controller-0
    lease-acquired: 10s ago
    lease-expires: 50s
model-leases:
  some-uuid:
    mysql:
      holder: mysql/0
      lease-acquired: 10s ago
      lease-expires: 50s
    wordpress:
      holder: wordpress/1
      lease-acquired: 10s ago
      lease-expires: 50s`[1:])
}

func (s *introspectionSuite) TestLeasesDataFilterSingleApp(c *gc.C) {
	s.setLeaseData()
	response := s.call(c, "/leases?app=keystone")
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	s.assertBody(c, response, `
model-leases:
  other-uuid:
    keystone:
      holder: keystone/42
      lease-acquired: 10s ago
      lease-expires: 50s`[1:])
}

func (s *introspectionSuite) TestLeasesDataFilterTwoApps(c *gc.C) {
	s.setLeaseData()
	response := s.call(c, "/leases?app=mysql&app=word")
	c.Assert(response.StatusCode, gc.Equals, http.StatusOK)
	s.assertBody(c, response, `
model-leases:
  other-uuid:
    mysql:
      holder: mysql/1
      lease-acquired: 10s ago
      lease-expires: 50s
  some-uuid:
    mysql:
      holder: mysql/0
      lease-acquired: 10s ago
      lease-expires: 50s
    wordpress:
      holder: wordpress/1
      lease-acquired: 10s ago
      lease-expires: 50s`[1:])
}

func (s *introspectionSuite) setLeaseData() {
	now := time.Date(2020, 8, 11, 15, 34, 23, 0, time.UTC)
	start := now.Add(-10 * time.Second)
	s.leases.data = &raftlease.Snapshot{
		Version:    1,
		GlobalTime: now,
		Entries: map[raftlease.SnapshotKey]raftlease.SnapshotEntry{
			raftlease.SnapshotKey{
				Namespace: lease.SingularControllerNamespace,
				ModelUUID: "some-uuid",
				Lease:     "some-uuid",
			}: raftlease.SnapshotEntry{
				Holder:   "controller-0",
				Start:    start,
				Duration: time.Minute,
			},
			raftlease.SnapshotKey{
				Namespace: lease.SingularControllerNamespace,
				ModelUUID: "other-uuid",
				Lease:     "other-uuid",
			}: raftlease.SnapshotEntry{
				Holder:   "controller-1",
				Start:    start,
				Duration: time.Minute,
			},
			raftlease.SnapshotKey{
				Namespace: lease.ApplicationLeadershipNamespace,
				ModelUUID: "some-uuid",
				Lease:     "mysql",
			}: raftlease.SnapshotEntry{
				Holder:   "mysql/0",
				Start:    start,
				Duration: time.Minute,
			},
			raftlease.SnapshotKey{
				Namespace: lease.ApplicationLeadershipNamespace,
				ModelUUID: "some-uuid",
				Lease:     "wordpress",
			}: raftlease.SnapshotEntry{
				Holder:   "wordpress/1",
				Start:    start,
				Duration: time.Minute,
			},
			raftlease.SnapshotKey{
				Namespace: lease.ApplicationLeadershipNamespace,
				ModelUUID: "other-uuid",
				Lease:     "mysql",
			}: raftlease.SnapshotEntry{
				Holder:   "mysql/1",
				Start:    start,
				Duration: time.Minute,
			},
			raftlease.SnapshotKey{
				Namespace: lease.ApplicationLeadershipNamespace,
				ModelUUID: "other-uuid",
				Lease:     "keystone",
			}: raftlease.SnapshotEntry{
				Holder:   "keystone/42",
				Start:    start,
				Duration: time.Minute,
			},
		},
	}
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
	}
}

func unixSocketHTTPTransport(socketPath string) *http.Transport {
	return &http.Transport{
		Dial: func(proto, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
}

type fakeLeases struct {
	err  error
	data *raftlease.Snapshot
}

func (f *fakeLeases) Snapshot() (raft.FSMSnapshot, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.data, nil
}
