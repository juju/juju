// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metrics_test

import (
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v5"

	"github.com/juju/juju/worker/uniter/metrics"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type MetricsBatchSuite struct {
}

var _ = gc.Suite(&MetricsBatchSuite{})

func (s *MetricsBatchSuite) TestAPIMetricBatch(c *gc.C) {
	batches := []metrics.MetricBatch{{
		CharmURL: "local:trusty/test-charm",
		UUID:     "test-uuid",
		Created:  time.Now(),
		Metrics: []jujuc.Metric{
			{
				Key:   "test-key-1",
				Value: "test-value-1",
				Time:  time.Now(),
			}, {
				Key:   "test-key-2",
				Value: "test-value-2",
				Time:  time.Now(),
			},
		},
	}, {
		CharmURL: "local:trusty/test-charm",
		UUID:     "test-uuid",
		Created:  time.Now(),
		Metrics:  []jujuc.Metric{},
	},
	}
	for _, batch := range batches {
		apiBatch := metrics.APIMetricBatch(batch)
		c.Assert(apiBatch.Batch.UUID, gc.DeepEquals, batch.UUID)
		c.Assert(apiBatch.Batch.CharmURL, gc.DeepEquals, batch.CharmURL)
		c.Assert(apiBatch.Batch.Created, gc.DeepEquals, batch.Created)
		c.Assert(len(apiBatch.Batch.Metrics), gc.Equals, len(batch.Metrics))
		for i, metric := range batch.Metrics {
			c.Assert(metric.Key, gc.DeepEquals, apiBatch.Batch.Metrics[i].Key)
			c.Assert(metric.Value, gc.DeepEquals, apiBatch.Batch.Metrics[i].Value)
			c.Assert(metric.Time, gc.DeepEquals, apiBatch.Batch.Metrics[i].Time)
		}
	}
}

func osDependentSockPath(c *gc.C) string {
	sockPath := filepath.Join(c.MkDir(), "test.sock")
	if runtime.GOOS == "windows" {
		return `\\.\pipe` + sockPath[2:]
	}
	return sockPath
}

// testPaths implements Paths for tests that do touch the filesystem.
type testPaths struct {
	tools        string
	charm        string
	socket       string
	metricsspool string
}

func newTestPaths(c *gc.C) testPaths {
	return testPaths{
		tools:        c.MkDir(),
		charm:        c.MkDir(),
		socket:       osDependentSockPath(c),
		metricsspool: c.MkDir(),
	}
}

func (p testPaths) GetMetricsSpoolDir() string {
	return p.metricsspool
}

func (p testPaths) GetToolsDir() string {
	return p.tools
}

func (p testPaths) GetCharmDir() string {
	return p.charm
}

func (p testPaths) GetJujucSocket() string {
	return p.socket
}

type MetricsRecorderSuite struct {
	testing.IsolationSuite

	paths   testPaths
	unitTag string
}

var _ = gc.Suite(&MetricsRecorderSuite{})

func (s *MetricsRecorderSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.paths = newTestPaths(c)
	s.unitTag = names.NewUnitTag("test-unit/0").String()
}

func (s *MetricsRecorderSuite) TestInit(c *gc.C) {
	w, err := metrics.NewJSONMetricRecorder(
		metrics.MetricRecorderConfig{
			SpoolDir: s.paths.GetMetricsSpoolDir(),
			Metrics:  map[string]corecharm.Metric{"pings": corecharm.Metric{}},
			CharmURL: "local:precise/wordpress",
			UnitTag:  s.unitTag,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	err = w.AddMetric("pings", "5", time.Now())
	c.Assert(err, jc.ErrorIsNil)
	err = w.Close()
	c.Assert(err, jc.ErrorIsNil)

	r, err := metrics.NewJSONMetricReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)
	batches, err := r.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 1)
	batch := batches[0]
	c.Assert(batch.CharmURL, gc.Equals, "local:precise/wordpress")
	c.Assert(batch.UUID, gc.Not(gc.Equals), "")
	c.Assert(batch.Metrics, gc.HasLen, 1)
	c.Assert(batch.Metrics[0].Key, gc.Equals, "pings")
	c.Assert(batch.Metrics[0].Value, gc.Equals, "5")
	c.Assert(batch.UnitTag, gc.Equals, s.unitTag)

	err = r.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricsRecorderSuite) TestUnknownMetricKey(c *gc.C) {
	w, err := metrics.NewJSONMetricRecorder(
		metrics.MetricRecorderConfig{
			SpoolDir: s.paths.GetMetricsSpoolDir(),
			Metrics:  map[string]corecharm.Metric{},
			CharmURL: "local:precise/wordpress",
			UnitTag:  s.unitTag,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	err = w.AddMetric("pings", "5", time.Now())
	c.Assert(err, gc.ErrorMatches, `metric key "pings" not declared by the charm`)
	err = w.Close()
	c.Assert(err, jc.ErrorIsNil)

	r, err := metrics.NewJSONMetricReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)
	batches, err := r.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 0)
}

type MetricsReaderSuite struct {
	paths   testPaths
	unitTag string

	w *metrics.JSONMetricRecorder
}

var _ = gc.Suite(&MetricsReaderSuite{})

func (s *MetricsReaderSuite) SetUpTest(c *gc.C) {
	s.paths = newTestPaths(c)
	s.unitTag = names.NewUnitTag("test-unit/0").String()

	var err error
	s.w, err = metrics.NewJSONMetricRecorder(
		metrics.MetricRecorderConfig{
			SpoolDir: s.paths.GetMetricsSpoolDir(),
			Metrics:  map[string]corecharm.Metric{"pings": corecharm.Metric{}},
			CharmURL: "local:precise/wordpress",
			UnitTag:  s.unitTag,
		})

	c.Assert(err, jc.ErrorIsNil)
	err = s.w.AddMetric("pings", "5", time.Now())
	c.Assert(err, jc.ErrorIsNil)
	err = s.w.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricsReaderSuite) TestTwoSimultaneousReaders(c *gc.C) {
	r, err := metrics.NewJSONMetricReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)

	r2, err := metrics.NewJSONMetricReader(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r2, gc.NotNil)
	err = r2.Close()
	c.Assert(err, jc.ErrorIsNil)
	err = r.Close()
	c.Assert(err, jc.ErrorIsNil)

}

func (s *MetricsReaderSuite) TestUnblockedReaders(c *gc.C) {
	r, err := metrics.NewJSONMetricReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)
	err = r.Close()
	c.Assert(err, jc.ErrorIsNil)

	r2, err := metrics.NewJSONMetricReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r2, gc.NotNil)
	err = r2.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricsReaderSuite) TestRemoval(c *gc.C) {
	r, err := metrics.NewJSONMetricReader(s.paths.GetMetricsSpoolDir())
	c.Assert(err, jc.ErrorIsNil)

	batches, err := r.Read()
	c.Assert(err, jc.ErrorIsNil)
	for _, batch := range batches {
		err := r.Remove(batch.UUID)
		c.Assert(err, jc.ErrorIsNil)
	}
	err = r.Close()
	c.Assert(err, jc.ErrorIsNil)

	batches, err = r.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 0)
	err = r.Close()
	c.Assert(err, jc.ErrorIsNil)
}
