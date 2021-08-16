// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package centralhub

import (
	"github.com/juju/testing"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"
)

type MetricsSuite struct {
	testing.IsolationSuite
	collector prometheus.Collector
}

var _ = gc.Suite(&MetricsSuite{})

func (s *MetricsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.collector = NewPubsubMetrics()
}

func (s *MetricsSuite) TestDescribe(c *gc.C) {
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
	c.Assert(descs[0].String(), gc.Matches, `.*fqName: "juju_pubsub_subscriptions".*`)
	c.Assert(descs[1].String(), gc.Matches, `.*fqName: "juju_pubsub_published".*`)
	c.Assert(descs[2].String(), gc.Matches, `.*fqName: "juju_pubsub_queue".*`)
	c.Assert(descs[3].String(), gc.Matches, `.*fqName: "juju_pubsub_consumed".*`)
}

func (s *MetricsSuite) TestCollect(c *gc.C) {
	ch := make(chan prometheus.Metric)
	go func() {
		defer close(ch)
		s.collector.Collect(ch)
	}()

	var metrics []prometheus.Metric
	for metric := range ch {
		metrics = append(metrics, metric)
	}
	c.Assert(metrics, gc.HasLen, 1)
}
