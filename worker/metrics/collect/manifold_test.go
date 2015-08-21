// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v5"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/metrics/collect"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	"github.com/juju/juju/worker/uniteravailability"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	dataDir  string
	oldLcAll string

	manifoldConfig collect.ManifoldConfig
	manifold       dependency.Manifold
	dummyResources dt.StubResources
	getResource    dependency.GetResourceFunc
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.manifoldConfig = collect.ManifoldConfig{
		AgentName:              "agent-name",
		APICallerName:          "apicaller-name",
		MetricSpoolName:        "metric-spool-name",
		UniterAvailabilityName: "uniteravailability-name",
	}
	s.manifold = collect.Manifold(s.manifoldConfig)

	s.dataDir = c.MkDir()
	toolsDir := tools.ToolsDir(s.dataDir, "unit-u-0")
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	// TODO(cmars) Move this ludicrous thing to a single place, where we can at
	// least reuse among uniter_test and other suites that need tools.
	cmd := exec.Command("go", "build", "github.com/juju/juju/cmd/jujud")
	cmd.Dir = toolsDir
	out, err := cmd.CombinedOutput()
	c.Logf(string(out))
	c.Assert(err, jc.ErrorIsNil)
	s.oldLcAll = os.Getenv("LC_ALL")
	os.Setenv("LC_ALL", "en_US")

	s.dummyResources = dt.StubResources{
		"agent-name":              dt.StubResource{Output: &dummyAgent{dataDir: s.dataDir}},
		"apicaller-name":          dt.StubResource{Output: &dummyAPICaller{}},
		"metric-spool-name":       dt.StubResource{Output: &dummyMetricFactory{}},
		"uniteravailability-name": dt.StubResource{Output: &dummyUniterAvailability{available: true}},
	}
	s.getResource = dt.StubGetResource(s.dummyResources)
}

func (s *ManifoldSuite) TearDownSuite(c *gc.C) {
	os.Setenv("LC_ALL", s.oldLcAll)
}

func (s *ManifoldSuite) SetCharm(c *gc.C, name, unitTag string) {
	paths := uniter.NewPaths(s.dataDir, names.NewUnitTag(unitTag))
	err := os.MkdirAll(filepath.Dir(paths.GetCharmDir()), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = fs.Copy(testcharms.Repo.CharmDirPath(name), paths.GetCharmDir())
	c.Assert(err, jc.ErrorIsNil)
	c.Log("charmdir=", paths.GetCharmDir())
}

// TestInputs ensures the collect manifold has the expected defined inputs.
func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{
		"agent-name", "apicaller-name", "metric-spool-name", "uniteravailability-name",
	})
}

// TestStartMissingDeps ensures that the manifold correctly handles a missing
// resource dependency.
func (s *ManifoldSuite) TestStartMissingDeps(c *gc.C) {
	for _, missingDep := range []string{
		"agent-name", "apicaller-name", "metric-spool-name", "uniteravailability-name",
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
	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := s.manifold.Start(getResource)
	c.Check(err, gc.IsNil)
	c.Check(worker, gc.NotNil)
	worker.Kill()
	err = worker.Wait()
	c.Assert(err, gc.IsNil)
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
		func(_ names.UnitTag, _ context.Paths, _ collect.UnitCharmLookup, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			return recorder, nil
		})
	collectEntity, err := (*collect.NewCollect)(s.manifoldConfig, s.getResource)
	c.Assert(err, gc.IsNil)
	err = collectEntity.Do(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(recorder.closed, jc.IsTrue)
	c.Assert(recorder.batches, gc.HasLen, 1)
	c.Assert(recorder.batches[0].CharmURL, gc.Equals, "cs:wordpress-37")
	c.Assert(recorder.batches[0].UnitTag, gc.Equals, "wp/0")
	c.Assert(recorder.batches[0].Metrics, gc.HasLen, 1)
	c.Assert(recorder.batches[0].Metrics[0].Key, gc.Equals, "juju-units")
	c.Assert(recorder.batches[0].Metrics[0].Value, gc.Equals, "1")
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
		func(_ names.UnitTag, _ context.Paths, _ collect.UnitCharmLookup, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			return recorder, nil
		})
	collectEntity, err := (*collect.NewCollect)(s.manifoldConfig, s.getResource)
	c.Assert(err, gc.IsNil)
	err = collectEntity.Do(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(recorder.closed, jc.IsTrue)
	c.Assert(recorder.batches, gc.HasLen, 0)
}

func (s *ManifoldSuite) TestDeclaredMetric(c *gc.C) {
	// TODO(cmars): need to have the runner execute an actual charm hook
	s.SetCharm(c, "metered", "u/0")
	recorder := &dummyRecorder{
		charmURL: "local:quantal/metered-1",
		unitTag:  "u/0",
		metrics: map[string]corecharm.Metric{
			"pings": corecharm.Metric{
				Type:        corecharm.MetricTypeGauge,
				Description: "pings-desc",
			},
		},
	}
	s.PatchValue(collect.NewRecorder,
		func(_ names.UnitTag, _ context.Paths, _ collect.UnitCharmLookup, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			return recorder, nil
		})
	collectEntity, err := (*collect.NewCollect)(s.manifoldConfig, s.getResource)
	c.Assert(err, gc.IsNil)
	err = collectEntity.Do(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(recorder.closed, jc.IsTrue)
	c.Assert(recorder.batches, gc.HasLen, 2)
}

func (s *ManifoldSuite) TestUndeclaredMetric(c *gc.C) {
	// TODO(cmars): need to have the runner execute an actual charm hook
	// that sends an undeclared metric.
	s.SetCharm(c, "metered", "u/0")
	recorder := &dummyRecorder{
		charmURL: "cs:metered-1",
		unitTag:  "u/0",
	}
	s.PatchValue(collect.NewRecorder,
		func(_ names.UnitTag, _ context.Paths, _ collect.UnitCharmLookup, _ spool.MetricFactory) (spool.MetricRecorder, error) {
			return recorder, nil
		})
	collectEntity, err := (*collect.NewCollect)(s.manifoldConfig, s.getResource)
	c.Assert(err, gc.IsNil)
	err = collectEntity.Do(nil)
	c.Assert(err, gc.IsNil)
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

type dummyAPICaller struct {
	base.APICaller
}

type dummyUniterAvailability struct {
	uniteravailability.UniterAvailabilityGetter

	available bool
}

func (a *dummyUniterAvailability) Available() bool {
	return a.available
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

	// outputs
	closed  bool
	batches []spool.MetricBatch
}

func (r *dummyRecorder) AddMetric(key, value string, created time.Time) error {
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
