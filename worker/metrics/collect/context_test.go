// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect_test

import (
	"runtime"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/keyvalues"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/worker/metrics/collect"
)

type ContextSuite struct {
	recorder *dummyRecorder
}

var _ = gc.Suite(&ContextSuite{})

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.recorder = &dummyRecorder{
		charmURL: "local:quantal/metered-1",
		unitTag:  "u/0",
		metrics: map[string]corecharm.Metric{
			"pings": corecharm.Metric{
				Type:        corecharm.MetricTypeGauge,
				Description: "pings-desc",
			},
		},
	}
}

func (s *ContextSuite) TestCtxDeclaredMetric(c *gc.C) {
	ctx := collect.NewHookContext("u/0", s.recorder)
	err := ctx.AddMetric("pings", "1", time.Now())
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.Flush("", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.recorder.closed, jc.IsTrue)
	c.Assert(s.recorder.batches, gc.HasLen, 1)
	c.Assert(s.recorder.batches[0].Metrics, gc.HasLen, 1)
	c.Assert(s.recorder.batches[0].Metrics[0].Key, gc.Equals, "pings")
	c.Assert(s.recorder.batches[0].Metrics[0].Value, gc.Equals, "1")
}

type dummyPaths struct{}

func (*dummyPaths) GetToolsDir() string             { return "/dummy/tools" }
func (*dummyPaths) GetCharmDir() string             { return "/dummy/charm" }
func (*dummyPaths) GetJujucSocket() string          { return "/dummy/jujuc.sock" }
func (*dummyPaths) GetMetricsSpoolDir() string      { return "/dummy/spool" }
func (*dummyPaths) ComponentDir(name string) string { return "/dummy/" + name }

func (s *ContextSuite) TestHookContextEnv(c *gc.C) {
	ctx := collect.NewHookContext("u/0", s.recorder)
	paths := &dummyPaths{}
	vars, err := ctx.HookVars(paths)
	c.Assert(err, jc.ErrorIsNil)
	varMap, err := keyvalues.Parse(vars, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(varMap["JUJU_AGENT_SOCKET"], gc.Equals, "/dummy/jujuc.sock")
	c.Assert(varMap["JUJU_UNIT_NAME"], gc.Equals, "u/0")
	key := "PATH"
	if runtime.GOOS == "windows" {
		key = "Path"
	}
	c.Assert(varMap[key], gc.Not(gc.Equals), "")
}
