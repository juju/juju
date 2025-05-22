// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package logsendermetrics_test

import (
	"testing"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/logsender/logsendermetrics"
	"github.com/juju/juju/internal/worker/logsender/logsendertest"
)

const maxLen = 3

type bufferedLogWriterSuite struct {
	testhelpers.IsolationSuite
	writer    *logsender.BufferedLogWriter
	collector logsendermetrics.BufferedLogWriterMetrics
}

func TestBufferedLogWriterSuite(t *testing.T) {
	tc.Run(t, &bufferedLogWriterSuite{})
}

func (s *bufferedLogWriterSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.writer = logsender.NewBufferedLogWriter(maxLen)
	s.collector = logsendermetrics.BufferedLogWriterMetrics{s.writer}
	s.AddCleanup(func(*tc.C) { s.writer.Close() })
}

func (s *bufferedLogWriterSuite) TestDescribe(c *tc.C) {
	ch := make(chan *prometheus.Desc)
	go func() {
		defer close(ch)
		s.collector.Describe(ch)
	}()
	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}
	c.Assert(descs, tc.HasLen, 4)
	c.Assert(descs[0].String(), tc.Matches, `.*fqName: "juju_logsender_capacity".*`)
	c.Assert(descs[1].String(), tc.Matches, `.*fqName: "juju_logsender_enqueued_total".*`)
	c.Assert(descs[2].String(), tc.Matches, `.*fqName: "juju_logsender_sent_total".*`)
	c.Assert(descs[3].String(), tc.Matches, `.*fqName: "juju_logsender_dropped_total".*`)
}

func (s *bufferedLogWriterSuite) TestCollect(c *tc.C) {
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
	c.Assert(metrics, tc.HasLen, 4)

	var dtoMetrics [4]*dto.Metric
	for i, metric := range metrics {
		dtoMetrics[i] = &dto.Metric{}
		err := metric.Write(dtoMetrics[i])
		c.Assert(err, tc.ErrorIsNil)
	}

	float64ptr := func(v float64) *float64 {
		return &v
	}
	c.Assert(dtoMetrics, tc.DeepEquals, [4]*dto.Metric{
		{Counter: &dto.Counter{Value: float64ptr(3)}},
		{Counter: &dto.Counter{Value: float64ptr(5)}},
		{Counter: &dto.Counter{Value: float64ptr(3)}},
		{Counter: &dto.Counter{Value: float64ptr(1)}},
	})
}
