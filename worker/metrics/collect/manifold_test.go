// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect_test

import (
	"os"
	"path/filepath"
	"time"

	corecharm "github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/metrics/collect"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type ManifoldSuite struct {
	coretesting.BaseSuite

	dataDir  string
	oldLcAll string

	manifoldConfig collect.ManifoldConfig
	manifold       dependency.Manifold
	resources      dt.StubResources
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
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

	s.resources = dt.StubResources{
		"agent-name":        dt.NewStubResource(&dummyAgent{dataDir: s.dataDir}),
		"metric-spool-name": dt.NewStubResource(&dummyMetricFactory{}),
		"charmdir-name":     dt.NewStubResource(&dummyCharmdir{aborted: false}),
	}
}

// TestInputs ensures the collect manifold has the expected defined inputs.
func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{
		"agent-name", "metric-spool-name", "charmdir-name",
	})
}

// TestStartMissingDeps ensures that the manifold correctly handles a missing
// resource dependency.
func (s *ManifoldSuite) TestStartMissingDeps(c *gc.C) {
	for _, missingDep := range []string{
		"agent-name", "metric-spool-name", "charmdir-name",
	} {
		testResources := dt.StubResources{}
		for k, v := range s.resources {
			if k == missingDep {
				testResources[k] = dt.StubResource{Error: dependency.ErrMissing}
			} else {
				testResources[k] = v
			}
		}
		worker, err := s.manifold.Start(testResources.Context())
		c.Check(worker, gc.IsNil)
		c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

// TestCollectWorkerStarts ensures that the manifold correctly sets up the worker.
func (s *ManifoldSuite) TestCollectWorkerStarts(c *gc.C) {
	s.PatchValue(collect.NewRecorder,
		func(_ names.UnitTag, _ context.Paths, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			// Return a dummyRecorder here, because otherwise a real one
			// *might* get instantiated and error out, if the periodic worker
			// happens to fire before the worker shuts down (as seen in
			// LP:#1497355).
			return &dummyRecorder{
				charmURL: "cs:ubuntu-1",
				unitTag:  "ubuntu/0",
			}, nil
		})
	s.PatchValue(collect.ReadCharm,
		func(_ names.UnitTag, _ context.Paths) (*corecharm.URL, map[string]corecharm.Metric, error) {
			return corecharm.MustParseURL("cs:ubuntu-1"), map[string]corecharm.Metric{"pings": {Description: "test metric", Type: corecharm.MetricTypeAbsolute}}, nil
		})
	worker, err := s.manifold.Start(s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestCollectWorkerErrorStopsListener(c *gc.C) {
	s.PatchValue(collect.NewRecorder,
		func(_ names.UnitTag, _ context.Paths, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			return nil, errors.New("blah")
		})
	listener := &mockListener{}
	s.PatchValue(collect.NewSocketListener, collect.NewSocketListenerFnc(listener))
	s.PatchValue(collect.ReadCharm,
		func(_ names.UnitTag, _ context.Paths) (*corecharm.URL, map[string]corecharm.Metric, error) {
			return corecharm.MustParseURL("local:ubuntu-1"), map[string]corecharm.Metric{"pings": {Description: "test metric", Type: corecharm.MetricTypeAbsolute}}, nil
		})
	worker, err := s.manifold.Start(s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	err = worker.Wait()
	c.Assert(err, gc.ErrorMatches, ".*blah")
	listener.CheckCallNames(c, "Stop")
}

type errorRecorder struct {
	*spool.JSONMetricRecorder
}

func (e *errorRecorder) AddMetric(
	key, value string, created time.Time, labels map[string]string) (err error) {
	return e.JSONMetricRecorder.AddMetric(key, "bad", created, labels)
}

func (s *ManifoldSuite) TestRecordMetricsError(c *gc.C) {
	// An error recording a metric does not propagate the error
	// to the worker which could cause a bounce.
	recorder, err := spool.NewJSONMetricRecorder(
		spool.MetricRecorderConfig{
			SpoolDir: c.MkDir(),
			Metrics: map[string]corecharm.Metric{
				"juju-units": {},
			},
			CharmURL: "local:precise/wordpress",
			UnitTag:  "unit-wordpress-0",
		})
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(collect.NewRecorder,
		func(_ names.UnitTag, _ context.Paths, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			return &errorRecorder{recorder}, nil
		})
	s.PatchValue(collect.ReadCharm,
		func(_ names.UnitTag, _ context.Paths) (*corecharm.URL, map[string]corecharm.Metric, error) {
			return corecharm.MustParseURL("cs:wordpress-37"), nil, nil
		})
	collectEntity, err := collect.NewCollect(s.manifoldConfig, s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	err = collectEntity.Do(nil)
	c.Assert(err, jc.ErrorIsNil)
}

// TestJujuUnitsBuiltinMetric tests that the juju-units built-in metric is collected
// with a mock implementation of newRecorder.
func (s *ManifoldSuite) TestJujuUnitsBuiltinMetric(c *gc.C) {
	recorder := &dummyRecorder{
		charmURL:         "cs:wordpress-37",
		unitTag:          "wp/0",
		isDeclaredMetric: true,
	}
	s.PatchValue(collect.NewRecorder,
		func(_ names.UnitTag, _ context.Paths, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			return recorder, nil
		})
	s.PatchValue(collect.ReadCharm,
		func(_ names.UnitTag, _ context.Paths) (*corecharm.URL, map[string]corecharm.Metric, error) {
			return corecharm.MustParseURL("cs:wordpress-37"), map[string]corecharm.Metric{"pings": {Description: "test metric", Type: corecharm.MetricTypeAbsolute}}, nil
		})
	collectEntity, err := collect.NewCollect(s.manifoldConfig, s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	err = collectEntity.Do(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(recorder.closed, jc.IsTrue)
	c.Assert(recorder.batches, gc.HasLen, 1)
	c.Assert(recorder.batches[0].CharmURL, gc.Equals, "cs:wordpress-37")
	c.Assert(recorder.batches[0].UnitTag, gc.Equals, "wp/0")
	c.Assert(recorder.batches[0].Metrics, gc.HasLen, 1)
	c.Assert(recorder.batches[0].Metrics[0].Key, gc.Equals, "juju-units")
	c.Assert(recorder.batches[0].Metrics[0].Value, gc.Equals, "1")
}

// TestAvailability tests that the charmdir resource is properly checked.
func (s *ManifoldSuite) TestAvailability(c *gc.C) {
	recorder := &dummyRecorder{
		charmURL:         "cs:wordpress-37",
		unitTag:          "wp/0",
		isDeclaredMetric: true,
	}
	s.PatchValue(collect.NewRecorder,
		func(_ names.UnitTag, _ context.Paths, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			return recorder, nil
		})
	s.PatchValue(collect.ReadCharm,
		func(_ names.UnitTag, _ context.Paths) (*corecharm.URL, map[string]corecharm.Metric, error) {
			return corecharm.MustParseURL("cs:wordpress-37"), map[string]corecharm.Metric{"pings": {Description: "test metric", Type: corecharm.MetricTypeAbsolute}}, nil
		})
	charmdir := &dummyCharmdir{}
	s.resources["charmdir-name"] = dt.NewStubResource(charmdir)
	collectEntity, err := collect.NewCollect(s.manifoldConfig, s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	charmdir.aborted = true
	err = collectEntity.Do(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(recorder.batches, gc.HasLen, 0)

	charmdir = &dummyCharmdir{aborted: false}
	s.resources["charmdir-name"] = dt.NewStubResource(charmdir)
	collectEntity, err = collect.NewCollect(s.manifoldConfig, s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	err = collectEntity.Do(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(recorder.closed, jc.IsTrue)
	c.Assert(recorder.batches, gc.HasLen, 1)
}

// TestNoMetricsDeclared tests that if metrics are not declared, none are
// collected, not even builtin.
func (s *ManifoldSuite) TestNoMetricsDeclared(c *gc.C) {
	recorder := &dummyRecorder{
		charmURL:         "cs:wordpress-37",
		unitTag:          "wp/0",
		isDeclaredMetric: false,
	}
	s.PatchValue(collect.NewRecorder,
		func(_ names.UnitTag, _ context.Paths, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			return recorder, nil
		})
	s.PatchValue(collect.ReadCharm,
		func(_ names.UnitTag, _ context.Paths) (*corecharm.URL, map[string]corecharm.Metric, error) {
			return corecharm.MustParseURL("cs:wordpress-37"), map[string]corecharm.Metric{}, nil
		})
	collectEntity, err := collect.NewCollect(s.manifoldConfig, s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	err = collectEntity.Do(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(recorder.closed, jc.IsTrue)
	c.Assert(recorder.batches, gc.HasLen, 0)
}

// TestCharmDirAborted tests when the fortress gating the charm directory
// aborts the collector.
func (s *ManifoldSuite) TestCharmDirAborted(c *gc.C) {
	charmdir := &dummyCharmdir{aborted: true}
	s.resources["charmdir-name"] = dt.NewStubResource(charmdir)
	_, err := collect.NewCollect(s.manifoldConfig, s.resources.Context())
	c.Assert(errors.Cause(err), gc.Equals, fortress.ErrAborted)
}

type dummyAgent struct {
	agent.Agent
	dataDir string
}

func (a dummyAgent) CurrentConfig() agent.Config {
	return &dummyAgentConfig{dataDir: a.dataDir}
}

type dummyAgentConfig struct {
	agent.Config
	dataDir string
}

// Tag implements agent.AgentConfig.
func (ac dummyAgentConfig) Tag() names.Tag {
	return names.NewUnitTag("u/0")
}

// DataDir implements agent.AgentConfig.
func (ac dummyAgentConfig) DataDir() string {
	return ac.dataDir
}

type dummyCharmdir struct {
	fortress.Guest

	aborted bool
}

func (a *dummyCharmdir) Visit(visit fortress.Visit, _ fortress.Abort) error {
	if a.aborted {
		return fortress.ErrAborted
	}
	return visit()
}

type dummyMetricFactory struct {
	spool.MetricFactory
}

type dummyRecorder struct {
	spool.MetricRecorder

	// inputs
	charmURL, unitTag string
	metrics           map[string]corecharm.Metric
	isDeclaredMetric  bool
	err               string

	// outputs
	closed  bool
	batches []spool.MetricBatch
}

func (r *dummyRecorder) AddMetric(
	key, value string, created time.Time, labels map[string]string) error {
	if r.err != "" {
		return errors.New(r.err)
	}
	then := time.Date(2015, 8, 20, 15, 48, 0, 0, time.UTC)
	r.batches = append(r.batches, spool.MetricBatch{
		CharmURL: r.charmURL,
		UUID:     utils.MustNewUUID().String(),
		Created:  then,
		Metrics: []jujuc.Metric{{
			Key:    key,
			Value:  value,
			Time:   then,
			Labels: labels,
		}},
		UnitTag: r.unitTag,
	})
	return nil
}

func (r *dummyRecorder) IsDeclaredMetric(key string) bool {
	if r.isDeclaredMetric {
		return true
	}
	_, ok := r.metrics[key]
	return ok
}

func (r *dummyRecorder) Close() error {
	r.closed = true
	return nil
}
