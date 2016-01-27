// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/agent"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
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
	dummyResources dt.StubResources
	getResource    dependency.GetResourceFunc
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

	s.dummyResources = dt.StubResources{
		"agent-name":        dt.StubResource{Output: &dummyAgent{dataDir: s.dataDir}},
		"metric-spool-name": dt.StubResource{Output: &dummyMetricFactory{}},
		"charmdir-name":     dt.StubResource{Output: &dummyCharmdir{aborted: false}},
	}
	s.getResource = dt.StubGetResource(s.dummyResources)
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
		for k, v := range s.dummyResources {
			if k == missingDep {
				testResources[k] = dt.StubResource{Error: dependency.ErrMissing}
			} else {
				testResources[k] = v
			}
		}
		getResource := dt.StubGetResource(testResources)
		worker, err := s.manifold.Start(getResource)
		c.Check(worker, gc.IsNil)
		c.Check(err, gc.Equals, dependency.ErrMissing)
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
			return corecharm.MustParseURL("cs:ubuntu-1"), map[string]corecharm.Metric{"pings": corecharm.Metric{Description: "test metric", Type: corecharm.MetricTypeAbsolute}}, nil
		})
	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	worker.Kill()
	err = worker.Wait()
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
			return corecharm.MustParseURL("cs:wordpress-37"), map[string]corecharm.Metric{"pings": corecharm.Metric{Description: "test metric", Type: corecharm.MetricTypeAbsolute}}, nil
		})
	collectEntity, err := collect.NewCollect(s.manifoldConfig, s.getResource)
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
			return corecharm.MustParseURL("cs:wordpress-37"), map[string]corecharm.Metric{"pings": corecharm.Metric{Description: "test metric", Type: corecharm.MetricTypeAbsolute}}, nil
		})
	charmdir := &dummyCharmdir{aborted: true}
	s.dummyResources["charmdir-name"] = dt.StubResource{Output: charmdir}
	getResource := dt.StubGetResource(s.dummyResources)
	collectEntity, err := collect.NewCollect(s.manifoldConfig, getResource)
	c.Assert(err, jc.ErrorIsNil)
	err = collectEntity.Do(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(recorder.batches, gc.HasLen, 0)

	charmdir = &dummyCharmdir{aborted: false}
	s.dummyResources["charmdir-name"] = dt.StubResource{Output: charmdir}
	getResource = dt.StubGetResource(s.dummyResources)
	collectEntity, err = collect.NewCollect(s.manifoldConfig, getResource)
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
	collectEntity, err := collect.NewCollect(s.manifoldConfig, s.getResource)
	c.Assert(err, jc.ErrorIsNil)
	err = collectEntity.Do(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(recorder.closed, jc.IsTrue)
	c.Assert(recorder.batches, gc.HasLen, 0)
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

func (r *dummyRecorder) AddMetric(key, value string, created time.Time) error {
	if r.err != "" {
		return errors.New(r.err)
	}
	then := time.Date(2015, 8, 20, 15, 48, 0, 0, time.UTC)
	r.batches = append(r.batches, spool.MetricBatch{
		CharmURL: r.charmURL,
		UUID:     utils.MustNewUUID().String(),
		Created:  then,
		Metrics: []jujuc.Metric{{
			Key:   key,
			Value: value,
			Time:  then,
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
