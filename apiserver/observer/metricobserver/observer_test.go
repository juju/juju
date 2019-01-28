// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"strconv"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/metricobserver"
	"github.com/juju/juju/apiserver/observer/metricobserver/mocks"
	"github.com/juju/juju/rpc"
)

type observerSuite struct {
	testing.IsolationSuite
	clock   *testclock.Clock
	factory observer.ObserverFactory
}

var _ = gc.Suite(&observerSuite{})

func (s *observerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.clock = testclock.NewClock(time.Time{})

	metricsCollector, finish := createMockMetricsWith(c, func(ctrl *gomock.Controller, counterVec *mocks.MockCounterVec, summaryVec *mocks.MockSummaryVec) {
		labels := prometheus.Labels{
			metricobserver.MetricLabelFacade:    "api-facade",
			metricobserver.MetricLabelVersion:   strconv.Itoa(42),
			metricobserver.MetricLabelMethod:    "api-method",
			metricobserver.MetricLabelErrorCode: "badness",
		}

		counter := mocks.NewMockCounter(ctrl)
		counter.EXPECT().Inc().Times(1)

		counterVec.EXPECT().With(labels).Return(counter).AnyTimes()

		summary := mocks.NewMockSummary(ctrl)
		summary.EXPECT().Observe(gomock.Any()).Times(1)

		summaryVec.EXPECT().With(labels).Return(summary).AnyTimes()
	})

	var err error
	s.factory, err = metricobserver.NewObserverFactory(metricobserver.Config{
		Clock:            s.clock,
		MetricsCollector: metricsCollector,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { finish() })
}

func (s *observerSuite) TestObserver(c *gc.C) {
	o := s.factory()
	c.Assert(o, gc.NotNil)
}

func (s *observerSuite) TestRPCObserver(c *gc.C) {
	o := s.factory().RPCObserver()
	c.Assert(o, gc.NotNil)

	latencies := []time.Duration{
		1000 * time.Millisecond,
		1500 * time.Millisecond,
		2000 * time.Millisecond,
	}
	for _, latency := range latencies {
		req := rpc.Request{
			Type:    "api-facade",
			Version: 42,
			Action:  "api-method",
		}
		o.ServerRequest(&rpc.Header{Request: req}, nil)
		s.clock.Advance(latency)
		o.ServerReply(req, &rpc.Header{ErrorCode: "badness"}, nil)
	}
}
