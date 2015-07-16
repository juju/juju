// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"sort"
	"time"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type AddMetricSuite struct {
	ContextSuite
}

var _ = gc.Suite(&AddMetricSuite{})

func (s *AddMetricSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, cmdString("add-metric"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
usage: add-metric key1=value1 [key2=value2 ...]
purpose: send metrics
`[1:])
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *AddMetricSuite) TestAddMetric(c *gc.C) {
	testCases := []struct {
		about         string
		cmd           []string
		canAddMetrics bool
		result        int
		stdout        string
		stderr        string
		expect        []jujuc.Metric
	}{
		{
			"add single metric",
			[]string{"add-metric", "key=50"},
			true,
			0,
			"",
			"",
			[]jujuc.Metric{{"key", "50", time.Now()}},
		}, {
			"no parameters error",
			[]string{"add-metric"},
			true,
			2,
			"",
			"error: no metrics specified\n",
			nil,
		}, {
			"invalid argument format",
			[]string{"add-metric", "key"},
			true,
			2,
			"",
			"error: expected \"key=value\", got \"key\"\n",
			nil,
		}, {
			"invalid argument format",
			[]string{"add-metric", "=key"},
			true,
			2,
			"",
			"error: expected \"key=value\", got \"=key\"\n",
			nil,
		}, {
			"invalid argument format, whitespace key",
			[]string{"add-metric", " =value"},
			true,
			2,
			"",
			"error: expected \"key=value\", got \"=value\"\n",
			nil,
		}, {
			"invalid argument format, whitespace key and value",
			[]string{"add-metric", " \t =  \n"},
			true,
			2,
			"",
			"error: expected \"key=value\", got \"=\"\n",
			nil,
		}, {
			"invalid argument format, whitespace value",
			[]string{"add-metric", " key =  "},
			true,
			2,
			"",
			"error: expected \"key=value\", got \"key=\"\n",
			nil,
		}, {
			"multiple metrics",
			[]string{"add-metric", "key=60", "key2=50.4"},
			true,
			0,
			"",
			"",
			[]jujuc.Metric{{"key", "60", time.Now()}, {"key2", "50.4", time.Now()}},
		}, {
			"multiple metrics, matching keys",
			[]string{"add-metric", "key=60", "key=50.4"},
			true,
			2,
			"",
			"error: key \"key\" specified more than once\n",
			nil,
		}, {
			"newline in metric value",
			[]string{"add-metric", "key=60\n", "key2\t=\t30", "\tkey3 =\t15"},
			true,
			0,
			"",
			"",
			[]jujuc.Metric{{"key", "60", time.Now()}, {"key2", "30", time.Now()}, {"key3", "15", time.Now()}},
		}, {
			"can't add metrics",
			[]string{"add-metric", "key=60", "key2=50.4"},
			false,
			1,
			"",
			"error: cannot record metric: metrics disabled\n",
			nil,
		}, {
			"cannot add builtin metric",
			[]string{"add-metric", "juju-key=50"},
			true,
			1,
			"",
			"error: juju-key uses a reserved prefix\n",
			nil,
		}}
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		hctx := s.GetHookContext(c, -1, "")
		hctx.canAddMetrics = t.canAddMetrics
		com, err := jujuc.NewCommand(hctx, cmdString(t.cmd[0]))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		ret := cmd.Main(com, ctx, t.cmd[1:])
		c.Check(ret, gc.Equals, t.result)
		c.Check(bufferString(ctx.Stdout), gc.Equals, t.stdout)
		c.Check(bufferString(ctx.Stderr), gc.Equals, t.stderr)
		c.Check(hctx.metrics, gc.HasLen, len(t.expect))

		sort.Sort(SortedMetrics(hctx.metrics))
		sort.Sort(SortedMetrics(t.expect))

		for i, expected := range t.expect {
			c.Check(expected.Key, gc.Equals, hctx.metrics[i].Key)
			c.Check(expected.Value, gc.Equals, hctx.metrics[i].Value)
		}
	}
}

type SortedMetrics []jujuc.Metric

func (m SortedMetrics) Len() int           { return len(m) }
func (m SortedMetrics) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m SortedMetrics) Less(i, j int) bool { return m[i].Key < m[j].Key }
