// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package apiserver_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
)

type apiservermetricsSuite struct {
	testing.IsolationSuite
	collector prometheus.Collector
}

var _ = gc.Suite(&apiservermetricsSuite{})

func (s *apiservermetricsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.collector = apiserver.NewMetricsCollector(&stubCollector{})
}

func (s *apiservermetricsSuite) TestDescribe(c *gc.C) {
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
	c.Assert(descs[0].String(), gc.Matches, `.*fqName: "juju_apiserver_connections_total".*`)
	c.Assert(descs[1].String(), gc.Matches, `.*fqName: "juju_apiserver_connection_count".*`)
	c.Assert(descs[2].String(), gc.Matches, `.*fqName: "juju_apiserver_connection_pause_seconds".*`)
	c.Assert(descs[3].String(), gc.Matches, `.*fqName: "juju_apiserver_active_login_attempts".*`)
}

func (s *apiservermetricsSuite) TestCollect(c *gc.C) {
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
		{Counter: &dto.Counter{Value: float64ptr(200)}},
		{Gauge: &dto.Gauge{Value: float64ptr(2)}},
		{Gauge: &dto.Gauge{Value: float64ptr(0.02)}},
		{Gauge: &dto.Gauge{Value: float64ptr(3)}},
	})
}

type stubCollector struct{}

func (a *stubCollector) TotalConnections() int64 {
	return 200
}

func (a *stubCollector) ConnectionCount() int64 {
	return 2
}

func (a *stubCollector) ConcurrentLoginAttempts() int64 {
	return 3
}

func (a *stubCollector) ConnectionPauseTime() time.Duration {
	return 20 * time.Millisecond
}
