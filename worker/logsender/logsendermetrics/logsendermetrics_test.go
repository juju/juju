// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package logsendermetrics_test

import (
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/logsender/logsendermetrics"
	"github.com/juju/juju/worker/logsender/logsendertest"
)

const maxLen = 3

type bufferedLogWriterSuite struct {
	testing.IsolationSuite
	writer    *logsender.BufferedLogWriter
	collector logsendermetrics.BufferedLogWriterMetrics
}

var _ = gc.Suite(&bufferedLogWriterSuite{})

func (s *bufferedLogWriterSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.writer = logsender.NewBufferedLogWriter(maxLen)
	s.collector = logsendermetrics.BufferedLogWriterMetrics{s.writer}
	s.AddCleanup(func(*gc.C) { s.writer.Close() })
}

func (s *bufferedLogWriterSuite) TestDescribe(c *gc.C) {
	ch := make(chan *prometheus.Desc)
	go func() {
		defer close(ch)
		s.collector.Describe(ch)
	}()
	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}
	c.Assert(descs, gc.HasLen, 4)
	c.Assert(descs[0].String(), gc.Matches, `.*fqName: "juju_logsender_capacity".*`)
	c.Assert(descs[1].String(), gc.Matches, `.*fqName: "juju_logsender_enqueued_total".*`)
	c.Assert(descs[2].String(), gc.Matches, `.*fqName: "juju_logsender_sent_total".*`)
	c.Assert(descs[3].String(), gc.Matches, `.*fqName: "juju_logsender_dropped_total".*`)
}

func (s *bufferedLogWriterSuite) TestCollect(c *gc.C) {
	s.writer.Write(loggo.Entry{})
	s.writer.Write(loggo.Entry{})
	s.writer.Write(loggo.Entry{})
	s.writer.Write(loggo.Entry{})
	s.writer.Write(loggo.Entry{}) // causes first to be dropped

	for i := 0; i < maxLen; i++ {
		<-s.writer.Logs()
	}

	logsendertest.ExpectLogStats(c, s.writer, logsender.LogStats{
		Enqueued: 5,
		Sent:     3,
		Dropped:  1,
	})

	ch := make(chan prometheus.Metric)
	go func() {
		defer close(ch)
		s.collector.Collect(ch)
	}()

	var metrics []prometheus.Metric
	for metric := range ch {
		metrics = append(metrics, metric)
	}
	c.Assert(metrics, gc.HasLen, 4)

	var dtoMetrics [4]dto.Metric
	for i, metric := range metrics {
		err := metric.Write(&dtoMetrics[i])
		c.Assert(err, jc.ErrorIsNil)
	}

	float64ptr := func(v float64) *float64 {
		return &v
	}
	c.Assert(dtoMetrics, jc.DeepEquals, [4]dto.Metric{
		{Counter: &dto.Counter{Value: float64ptr(3)}},
		{Counter: &dto.Counter{Value: float64ptr(5)}},
		{Counter: &dto.Counter{Value: float64ptr(3)}},
		{Counter: &dto.Counter{Value: float64ptr(1)}},
	})
}
