// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect_test

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v6-unstable"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/metrics/collect"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner/context"
)

type listenerSuite struct {
	coretesting.BaseSuite

	manifoldConfig collect.ManifoldConfig
	manifold       dependency.Manifold
	dataDir        string
	dummyResources dt.StubResources
	getResource    dependency.GetResourceFunc
}

var _ = gc.Suite(&listenerSuite{})

func (s *listenerSuite) SetUpTest(c *gc.C) {
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

	s.dummyResources = dt.StubResources{
		"agent-name":        dt.StubResource{Output: &dummyAgent{dataDir: s.dataDir}},
		"metric-spool-name": dt.StubResource{Output: &dummyMetricFactory{}},
		"charmdir-name":     dt.StubResource{Output: &dummyCharmdir{aborted: false}},
	}
	s.getResource = dt.StubGetResource(s.dummyResources)
}

func (s *listenerSuite) TestListenerStart(c *gc.C) {
	s.PatchValue(collect.NewRecorder,
		func(_ names.UnitTag, _ context.Paths, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			// Return a dummyRecorder here, because otherwise a real one
			// *might* get instantiated and error out, if the periodic worker
			// happens to fire before the worker shuts down (as seen in
			// LP:#1497355).
			return &dummyRecorder{
				charmURL: "local:trusty/metered-1",
				unitTag:  "metered/0",
			}, nil
		})
	s.PatchValue(collect.ReadCharm,
		func(_ names.UnitTag, _ context.Paths) (*corecharm.URL, map[string]corecharm.Metric, error) {
			return corecharm.MustParseURL("local:trusty/metered-1"), map[string]corecharm.Metric{"pings": corecharm.Metric{Description: "test metric", Type: corecharm.MetricTypeAbsolute}}, nil
		})
	s.PatchValue(collect.SocketPath,
		func(p uniter.Paths) string {
			return filepath.Join(s.dataDir, "metrics-collect.socket")
		})
	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *listenerSuite) TestJujuUnitsBuiltinMetric(c *gc.C) {
	recorder := &dummyRecorder{
		charmURL: "local:trusty/metered-1",
		unitTag:  "metered/0",
	}
	s.PatchValue(collect.NewRecorder,
		func(_ names.UnitTag, _ context.Paths, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			// Return a dummyRecorder here, because otherwise a real one
			// *might* get instantiated and error out, if the periodic worker
			// happens to fire before the worker shuts down (as seen in
			// LP:#1497355).
			return recorder, nil
		})
	s.PatchValue(collect.ReadCharm,
		func(_ names.UnitTag, _ context.Paths) (*corecharm.URL, map[string]corecharm.Metric, error) {
			return corecharm.MustParseURL("local:trusty/metered-1"), map[string]corecharm.Metric{
				"pings": corecharm.Metric{
					Description: "test metric",
					Type:        corecharm.MetricTypeAbsolute,
				},
				"juju-units": corecharm.Metric{
					Description: "built in metric",
					Type:        corecharm.MetricTypeAbsolute,
				},
			}, nil
		})
	s.PatchValue(collect.SocketPath,
		func(p uniter.Paths) string {
			return filepath.Join(s.dataDir, "metrics-collect.socket")
		})
	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)

	conn, err := net.Dial("unix", filepath.Join(s.dataDir, "metrics-collect.socket"))
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	var batches []spool.MetricBatch
	err = decoder.Decode(&batches)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 1)

	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
}
