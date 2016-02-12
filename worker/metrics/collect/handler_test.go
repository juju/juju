// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect_test

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v6-unstable"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/metrics/collect"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter/runner/context"
)

type handlerSuite struct {
	coretesting.BaseSuite

	manifoldConfig collect.ManifoldConfig
	manifold       dependency.Manifold
	dataDir        string
	dummyResources dt.StubResources
	getResource    dependency.GetResourceFunc
	recorder       *dummyRecorder
	listener       *mockListener
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
			"pings": corecharm.Metric{
				Description: "test metric",
				Type:        corecharm.MetricTypeAbsolute,
			},
			"juju-units": corecharm.Metric{},
		},
	}

	s.dummyResources = dt.StubResources{
		"agent-name":        dt.StubResource{Output: &dummyAgent{dataDir: s.dataDir}},
		"metric-spool-name": dt.StubResource{Output: &mockMetricFactory{recorder: s.recorder}},
		"charmdir-name":     dt.StubResource{Output: &dummyCharmdir{aborted: false}},
	}
	s.getResource = dt.StubGetResource(s.dummyResources)

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
					"pings": corecharm.Metric{
						Description: "test metric",
						Type:        corecharm.MetricTypeAbsolute,
					},
					"juju-units": corecharm.Metric{},
				},
			}, nil
		},
	)
	s.PatchValue(collect.ReadCharm,
		func(_ names.UnitTag, _ context.Paths) (*corecharm.URL, map[string]corecharm.Metric, error) {
			return corecharm.MustParseURL("local:trusty/metered-1"),
				map[string]corecharm.Metric{
					"pings":      corecharm.Metric{Description: "test metric", Type: corecharm.MetricTypeAbsolute},
					"juju-units": corecharm.Metric{},
				}, nil
		},
	)
	s.listener = &mockListener{}
	s.PatchValue(collect.NewSocketListener, collect.NewSocketListenerFnc(s.listener))
}

func (s *handlerSuite) TestListenerStart(c *gc.C) {
	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	c.Assert(s.listener.Calls(), gc.HasLen, 0)
	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
	s.listener.CheckCall(c, 0, "Stop")
}

func (s *handlerSuite) TestJujuUnitsBuiltinMetric(c *gc.C) {
	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	c.Assert(s.listener.Calls(), gc.HasLen, 0)

	conn, err := s.listener.trigger()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conn.Calls(), gc.HasLen, 3)
	conn.CheckCall(c, 2, "Close")

	responseString := strings.Trim(string(conn.data), " \n\t")
	c.Assert(responseString, gc.Equals, "ok")
	c.Assert(s.recorder.batches, gc.HasLen, 1)

	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
	s.listener.CheckCall(c, 0, "Stop")
}

func (s *handlerSuite) TestHandlerError(c *gc.C) {
	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	c.Assert(s.listener.Calls(), gc.HasLen, 0)

	s.recorder.err = "well, this is embarassing"

	conn, err := s.listener.trigger()
	c.Assert(err, gc.ErrorMatches, "failed to collect metrics: error adding 'juju-units' metric: well, this is embarassing")
	c.Assert(conn.Calls(), gc.HasLen, 3)
	conn.CheckCall(c, 2, "Close")

	responseString := strings.Trim(string(conn.data), " \n\t")
	c.Assert(responseString, gc.Matches, ".*well, this is embarassing")
	c.Assert(s.recorder.batches, gc.HasLen, 0)

	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
	s.listener.CheckCall(c, 0, "Stop")
}

type mockListener struct {
	testing.Stub
	handler spool.ConnectionHandler
}

func (l *mockListener) trigger() (*mockConnection, error) {
	conn := &mockConnection{}
	err := l.handler.Handle(conn)
	if err != nil {
		return conn, err
	}
	return conn, nil
}

// Stop implements the stopper interface.
func (l *mockListener) Stop() {
	l.AddCall("Stop")
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
	c.data = data
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
