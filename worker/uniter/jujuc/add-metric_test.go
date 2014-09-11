// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"time"

	"github.com/juju/cmd"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/jujuc"
)

type AddMetricSuite struct {
	ContextSuite
}

var _ = gc.Suite(&AddMetricSuite{})

func (s *AddMetricSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "add-metric")
	c.Assert(err, gc.IsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
usage: add-metric key=value [key=value ...]
purpose: send metrics
`[1:])
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *AddMetricSuite) TestAddMetric(c *gc.C) {
	testCases := []struct {
		about  string
		cmd    []string
		result int
		stdout string
		stderr string
		expect []jujuc.Metric
	}{
		{
			"add single metric",
			[]string{"add-metric", "key=50"},
			0,
			"",
			"",
			[]jujuc.Metric{{"key", "50", time.Now()}},
		}, {
			"no parameters error",
			[]string{"add-metric"},
			2,
			"",
			"error: no metrics specified\n",
			nil,
		}, {
			"invalid metric value",
			[]string{"add-metric", "key=invalidvalue"},
			2,
			"",
			"error: invalid value type: expected float, got \"invalidvalue\"\n",
			nil,
		}, {
			"invalid argument format",
			[]string{"add-metric", "key"},
			2,
			"",
			"error: expected \"key=value\", got \"key\"\n",
			nil,
		}, {
			"invalid argument format",
			[]string{"add-metric", "=key"},
			2,
			"",
			"error: expected \"key=value\", got \"=key\"\n",
			nil,
		}, {
			"multiple metrics",
			[]string{"add-metric", "key=60", "key2=50.4"},
			0,
			"",
			"",
			[]jujuc.Metric{{"key", "60", time.Now()}, {"key2", "50.4", time.Now()}},
		}, {
			"multiple metrics, matching keys",
			[]string{"add-metric", "key=60", "key=50.4"},
			0,
			"",
			"",
			[]jujuc.Metric{{"key", "60", time.Now()}, {"key", "50.4", time.Now()}},
		},
	}
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, t.cmd[0])
		c.Assert(err, gc.IsNil)
		ctx := testing.Context(c)
		ret := cmd.Main(com, ctx, t.cmd[1:])
		c.Assert(ret, gc.Equals, t.result)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, t.stdout)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, t.stderr)
		c.Assert(len(hctx.metrics), gc.Equals, len(t.expect))
		if len(t.expect) > 0 {
			for i, expected := range t.expect {
				c.Assert(expected.Key, gc.Equals, hctx.metrics[i].Key)
				c.Assert(expected.Value, gc.Equals, hctx.metrics[i].Value)
			}
		}
	}
}
