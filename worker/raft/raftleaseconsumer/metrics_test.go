// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftleaseconsumer

import (
	time "time"

	gomock "github.com/golang/mock/gomock"
	"github.com/juju/testing"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"
)

type MetricsSuite struct {
	testing.IsolationSuite
	collector prometheus.Collector
}

var _ = gc.Suite(&MetricsSuite{})

func (s *MetricsSuite) TestDescribe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan *prometheus.Desc)
	go func() {
		defer close(ch)
		s.collector.Describe(ch)
	}()
	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}
	c.Assert(descs, gc.HasLen, 1)
	c.Assert(descs[0].String(), gc.Matches, `.*fqName: "juju_raftleaseconsumer_request".*`)
}

func (s *MetricsSuite) TestCollect(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan prometheus.Metric)
	go func() {
		defer close(ch)
		s.collector.Collect(ch)
	}()

	var metrics []prometheus.Metric
	for metric := range ch {
		metrics = append(metrics, metric)
	}
	c.Assert(metrics, gc.HasLen, 0)
}

func (s *MetricsSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now()).AnyTimes()

	s.collector = newMetricsCollector(clock)

	return ctrl
}
