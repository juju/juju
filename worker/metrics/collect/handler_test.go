// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect_test

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	corecharm "github.com/juju/charm/v7"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/metrics/collect"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter/runner/context"
)

type handlerSuite struct {
	coretesting.BaseSuite

	manifoldConfig collect.ManifoldConfig
	manifold       dependency.Manifold
	dataDir        string
	resources      dt.StubResources
	recorder       *dummyRecorder
	listener       *mockListener
	mockReadCharm  *mockReadCharm
}

var _ = gc.Suite(&handlerSuite{})

func (s *handlerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.manifoldConfig = collect.ManifoldConfig{
		AgentName:       "agent-name",
		MetricSpoolName: "metric-spool-name",
		CharmDirName:    "charmdir-name",
	}
	s.manifold = collect.Manifold(s.manifoldConfig)
	s.dataDir = c.MkDir()

	// create unit agent base dir so that hooks can run.
	err := os.MkdirAll(filepath.Join(s.dataDir, "agents", "unit-u-0"), 0777)
	c.Assert(err, jc.ErrorIsNil)

	s.recorder = &dummyRecorder{
		charmURL: "local:trusty/metered-1",
		unitTag:  "metered/0",
		metrics: map[string]corecharm.Metric{
			"pings": {
				Description: "test metric",
				Type:        corecharm.MetricTypeAbsolute,
			},
			"juju-units": {},
		},
	}

	s.resources = dt.StubResources{
		"agent-name":        dt.NewStubResource(&dummyAgent{dataDir: s.dataDir}),
		"metric-spool-name": dt.NewStubResource(&mockMetricFactory{recorder: s.recorder}),
		"charmdir-name":     dt.NewStubResource(&dummyCharmdir{aborted: false}),
	}

	s.PatchValue(collect.NewRecorder,
		func(_ names.UnitTag, _ context.Paths, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			// Return a dummyRecorder here, because otherwise a real one
			// *might* get instantiated and error out, if the periodic worker
			// happens to fire before the worker shuts down (as seen in
			// LP:#1497355).
			return &dummyRecorder{
				charmURL: "local:trusty/metered-1",
				unitTag:  "metered/0",
				metrics: map[string]corecharm.Metric{
					"pings": {
						Description: "test metric",
						Type:        corecharm.MetricTypeAbsolute,
					},
					"juju-units": {},
				},
			}, nil
		},
	)
	s.mockReadCharm = &mockReadCharm{}
	s.PatchValue(collect.ReadCharm, s.mockReadCharm.ReadCharm)
	s.listener = &mockListener{}
	s.PatchValue(collect.NewSocketListener, collect.NewSocketListenerFnc(s.listener))
}

func (s *handlerSuite) TestListenerStart(c *gc.C) {
	worker, err := s.manifold.Start(s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	c.Assert(s.listener.Calls(), gc.HasLen, 0)
	workertest.CleanKill(c, worker)
	s.listener.CheckCall(c, 0, "Stop")
}

func (s *handlerSuite) TestJujuUnitsBuiltinMetric(c *gc.C) {
	worker, err := s.manifold.Start(s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	c.Assert(s.listener.Calls(), gc.HasLen, 0)

	conn, err := s.listener.trigger()
	c.Assert(err, jc.ErrorIsNil)
	conn.CheckCallNames(c, "SetDeadline", "Write", "Close")

	responseString := strings.Trim(string(conn.data), " \n\t")
	c.Assert(responseString, gc.Equals, "ok")
	c.Assert(s.recorder.batches, gc.HasLen, 1)

	workertest.CleanKill(c, worker)
	s.listener.CheckCall(c, 0, "Stop")
}

func (s *handlerSuite) TestReadCharmCalledOnEachTrigger(c *gc.C) {
	worker, err := s.manifold.Start(s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	c.Assert(s.listener.Calls(), gc.HasLen, 0)

	_, err = s.listener.trigger()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.listener.trigger()
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(collect.ReadCharm, s.mockReadCharm.ReadCharm)
	workertest.CleanKill(c, worker)

	// Expect 3 calls to ReadCharm, one on start and one per handler call
	s.mockReadCharm.CheckCallNames(c, "ReadCharm", "ReadCharm", "ReadCharm")
	s.listener.CheckCall(c, 0, "Stop")
}

func (s *handlerSuite) TestHandlerError(c *gc.C) {
	worker, err := s.manifold.Start(s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	c.Assert(s.listener.Calls(), gc.HasLen, 0)

	s.recorder.err = "well, this is embarrassing"

	conn, err := s.listener.trigger()
	c.Assert(err, gc.ErrorMatches, "failed to collect metrics: error adding 'juju-units' metric: well, this is embarrassing")
	conn.CheckCallNames(c, "SetDeadline", "Write", "Close")

	responseString := strings.Trim(string(conn.data), " \n\t")
	//c.Assert(responseString, gc.Matches, ".*well, this is embarrassing")
	c.Assert(responseString, gc.Equals, `error: failed to collect metrics: error adding 'juju-units' metric: well, this is embarrassing`)
	c.Assert(s.recorder.batches, gc.HasLen, 0)

	workertest.CleanKill(c, worker)
	s.listener.CheckCall(c, 0, "Stop")
}

type mockListener struct {
	testing.Stub
	handler spool.ConnectionHandler
}

func (l *mockListener) trigger() (*mockConnection, error) {
	conn := &mockConnection{}
	dying := make(chan struct{})
	err := l.handler.Handle(conn, dying)
	if err != nil {
		return conn, err
	}
	return conn, nil
}

// Stop implements the stopper interface.
func (l *mockListener) Stop() error {
	l.AddCall("Stop")
	return nil
}

func (l *mockListener) SetHandler(handler spool.ConnectionHandler) {
	l.handler = handler
}

type mockConnection struct {
	net.Conn
	testing.Stub
	data []byte
}

// SetDeadline implements the net.Conn interface.
func (c *mockConnection) SetDeadline(t time.Time) error {
	c.AddCall("SetDeadline", t)
	return nil
}

// Write implements the net.Conn interface.
func (c *mockConnection) Write(data []byte) (int, error) {
	c.AddCall("Write", data)
	c.data = make([]byte, len(data))
	copy(c.data, data)
	return len(data), nil
}

// Close implements the net.Conn interface.
func (c *mockConnection) Close() error {
	c.AddCall("Close")
	return nil
}

type mockMetricFactory struct {
	spool.MetricFactory
	recorder *dummyRecorder
}

// Recorder implements the spool.MetricFactory interface.
func (f *mockMetricFactory) Recorder(metrics map[string]corecharm.Metric, charmURL, unitTag string) (spool.MetricRecorder, error) {
	return f.recorder, nil
}

type mockReadCharm struct {
	testing.Stub
}

func (m *mockReadCharm) ReadCharm(unitTag names.UnitTag, paths context.Paths) (*corecharm.URL, map[string]corecharm.Metric, error) {
	m.MethodCall(m, "ReadCharm", unitTag, paths)
	return corecharm.MustParseURL("local:trusty/metered-1"),
		map[string]corecharm.Metric{
			"pings":      {Description: "test metric", Type: corecharm.MetricTypeAbsolute},
			"juju-units": {},
		}, nil
}
