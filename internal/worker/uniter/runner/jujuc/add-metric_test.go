// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"sort"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type AddMetricSuite struct {
	ContextSuite
}

var _ = gc.Suite(&AddMetricSuite{})

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
			[]jujuc.Metric{{Key: "key", Value: "50", Time: time.Now()}},
		}, {
			"no parameters error",
			[]string{"add-metric"},
			true,
			2,
			"",
			"ERROR no metrics specified\n",
			nil,
		}, {
			"invalid argument format",
			[]string{"add-metric", "key"},
			true,
			2,
			"",
			"ERROR invalid metrics: expected \"key=value\", got \"key\"\n",
			nil,
		}, {
			"invalid argument format",
			[]string{"add-metric", "=key"},
			true,
			2,
			"",
			"ERROR invalid metrics: expected \"key=value\", got \"=key\"\n",
			nil,
		}, {
			"invalid argument format, whitespace key",
			[]string{"add-metric", " =value"},
			true,
			2,
			"",
			"ERROR invalid metrics: expected \"key=value\", got \"=value\"\n",
			nil,
		}, {
			"invalid argument format, whitespace key and value",
			[]string{"add-metric", " \t =  \n"},
			true,
			2,
			"",
			"ERROR invalid metrics: expected \"key=value\", got \"=\"\n",
			nil,
		}, {
			"invalid argument format, whitespace value",
			[]string{"add-metric", " key =  "},
			true,
			2,
			"",
			"ERROR invalid metrics: expected \"key=value\", got \"key=\"\n",
			nil,
		}, {
			"multiple metrics",
			[]string{"add-metric", "key=60", "key2=50.4"},
			true,
			0,
			"",
			"",
			[]jujuc.Metric{{
				Key:   "key",
				Value: "60",
				Time:  time.Now(),
			}, {
				Key:   "key2",
				Value: "50.4",
				Time:  time.Now(),
			}},
		}, {
			"multiple metrics, matching keys",
			[]string{"add-metric", "key=60", "key=50.4"},
			true,
			2,
			"",
			"ERROR invalid metrics: key \"key\" specified more than once\n",
			nil,
		}, {
			"newline in metric value",
			[]string{"add-metric", "key=60\n", "key2\t=\t30", "\tkey3 =\t15"},
			true,
			0,
			"",
			"",
			[]jujuc.Metric{{
				Key: "key", Value: "60", Time: time.Now(),
			}, {
				Key: "key2", Value: "30", Time: time.Now(),
			}, {
				Key: "key3", Value: "15", Time: time.Now(),
			}},
		}, {
			"can't add metrics",
			[]string{"add-metric", "key=60", "key2=50.4"},
			false,
			1,
			"",
			"ERROR cannot record metric: metrics disabled\n",
			nil,
		}, {
			"cannot add builtin metric",
			[]string{"add-metric", "juju-key=50"},
			true,
			1,
			"",
			"ERROR juju-key uses a reserved prefix\n",
			nil,
		}, {
			"invalid label format",
			[]string{"add-metric", "--labels", "foo", "key=1"},
			true,
			2,
			"",
			"ERROR invalid labels: expected \"key=value\", got \"foo\"\n",
			nil,
		}, {
			"invalid label format",
			[]string{"add-metric", "--labels", "=bar", "key=1"},
			true,
			2,
			"",
			"ERROR invalid labels: expected \"key=value\", got \"=bar\"\n",
			nil,
		}, {
			"invalid label format, whitespace key",
			[]string{"add-metric", "--labels", " =bar", "key=1"},
			true,
			2,
			"",
			"ERROR invalid labels: expected \"key=value\", got \"=bar\"\n",
			nil,
		}, {
			"invalid label format, whitespace key and value",
			[]string{"add-metric", "--labels", " \t =  \n", "key=1"},
			true,
			2,
			"",
			"ERROR invalid labels: expected \"key=value\", got \"=\"\n",
			nil,
		}, {
			"invalid label format, whitespace value",
			[]string{"add-metric", "--labels", " foo =  ", "key=1"},
			true,
			2,
			"",
			"ERROR invalid labels: expected \"key=value\", got \"foo=\"\n",
			nil,
		}, {
			"add single metric with label",
			[]string{"add-metric", "--labels", "foo=bar", "key=50"},
			true,
			0,
			"",
			"",
			[]jujuc.Metric{{
				Key: "key", Value: "50", Time: time.Now(),
				Labels: map[string]string{"foo": "bar"},
			}},
		}, {
			"add single metric with labels",
			[]string{"add-metric", "--labels", "foo=bar,baz=quux", "key=510"},
			true,
			0,
			"",
			"",
			[]jujuc.Metric{{
				Key: "key", Value: "510", Time: time.Now(),
				Labels: map[string]string{"foo": "bar", "baz": "quux"},
			}},
		}, {
			"add single metric with labels, whitespace",
			[]string{"add-metric", "--labels", " foo = bar, baz = quux ", "key=510"},
			true,
			0,
			"",
			"",
			[]jujuc.Metric{{
				Key: "key", Value: "510", Time: time.Now(),
				Labels: map[string]string{"foo": "bar", "baz": "quux"},
			}},
		}, {
			"add multiple metrics with labels",
			[]string{"add-metric", "--labels", "foo=bar,baz=quux", "a=1", "b=2"},
			true,
			0,
			"",
			"",
			[]jujuc.Metric{{
				Key: "a", Value: "1", Time: time.Now(),
				Labels: map[string]string{"foo": "bar", "baz": "quux"},
			}, {
				Key: "b", Value: "2", Time: time.Now(),
				Labels: map[string]string{"foo": "bar", "baz": "quux"},
			}},
		}, {
			"can't add metrics with labels",
			[]string{"add-metric", "--labels", "foo=bar", "key=60", "key2=50.4"},
			false,
			1,
			"",
			"ERROR cannot record metric: metrics disabled\n",
			nil,
		}, {
			"cannot add builtin metric with labels",
			[]string{"add-metric", "--labels", "foo=bar", "juju-key=50"},
			true,
			1,
			"",
			"ERROR juju-key uses a reserved prefix\n",
			nil,
		}}
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		hctx := s.GetHookContext(c, -1, "")
		hctx.canAddMetrics = t.canAddMetrics
		com, err := jujuc.NewHookCommand(hctx, t.cmd[0])
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		ret := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.cmd[1:])
		c.Check(ret, gc.Equals, t.result)
		c.Check(bufferString(ctx.Stdout), gc.Equals, t.stdout)
		c.Check(bufferString(ctx.Stderr), gc.Equals, t.stderr)
		c.Check(hctx.metrics, gc.HasLen, len(t.expect))
		if len(hctx.metrics) != len(t.expect) {
			continue
		}

		sort.Sort(SortedMetrics(hctx.metrics))
		sort.Sort(SortedMetrics(t.expect))

		for i, expected := range t.expect {
			c.Check(expected.Key, gc.Equals, hctx.metrics[i].Key)
			c.Check(expected.Value, gc.Equals, hctx.metrics[i].Value)
			c.Check(expected.Labels, gc.DeepEquals, hctx.metrics[i].Labels)
		}
	}
}

type SortedMetrics []jujuc.Metric

func (m SortedMetrics) Len() int { return len(m) }

func (m SortedMetrics) Swap(i, j int) { m[i], m[j] = m[j], m[i] }

func (m SortedMetrics) Less(i, j int) bool { return m[i].Key < m[j].Key }
