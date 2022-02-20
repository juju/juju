// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"strconv"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/metricobserver"
	"github.com/juju/juju/rpc"
)

type observerSuite struct {
	testing.IsolationSuite
	clock *testclock.Clock
}

var _ = gc.Suite(&observerSuite{})

func (s *observerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.clock = testclock.NewClock(time.Time{})
}

func (s *observerSuite) TestObserver(c *gc.C) {
	factory, finish := s.createFactory(c)
	defer finish()

	o := factory()
	c.Assert(o, gc.NotNil)
}

func (s *observerSuite) TestRPCObserver(c *gc.C) {
	factory, finish := s.createFactory(c)
	defer finish()

	o := factory().RPCObserver()
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

func (s *observerSuite) createFactory(c *gc.C) (observer.ObserverFactory, func()) {
	metricsCollector, finish := createMockMetrics(c, prometheus.Labels{
		metricobserver.MetricLabelFacade:    "api-facade",
		metricobserver.MetricLabelVersion:   strconv.Itoa(42),
		metricobserver.MetricLabelMethod:    "api-method",
		metricobserver.MetricLabelErrorCode: "error",
	})

	factory, err := metricobserver.NewObserverFactory(metricobserver.Config{
		Clock:            s.clock,
		MetricsCollector: metricsCollector,
	})
	c.Assert(err, jc.ErrorIsNil)
	return factory, finish
}
