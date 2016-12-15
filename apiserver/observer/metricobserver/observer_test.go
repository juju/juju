// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/metricobserver"
	"github.com/juju/juju/rpc"
)

type observerSuite struct {
	testing.IsolationSuite
	clock    *testing.Clock
	registry *prometheus.Registry
	factory  observer.ObserverFactory
}

var _ = gc.Suite(&observerSuite{})

func (s *observerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.clock = testing.NewClock(time.Time{})
	s.registry = prometheus.NewPedanticRegistry()

	var err error
	s.factory, err = metricobserver.NewObserverFactory(metricobserver.Config{
		Clock:                s.clock,
		PrometheusRegisterer: s.registry,
	})
	c.Assert(err, jc.ErrorIsNil)
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

	stringptr := func(s string) *string {
		return &s
	}
	metricTypePtr := func(t dto.MetricType) *dto.MetricType {
		return &t
	}
	float64ptr := func(f float64) *float64 {
		return &f
	}
	uint64ptr := func(u uint64) *uint64 {
		return &u
	}

	labels := []*dto.LabelPair{
		{stringptr("error_code"), stringptr("badness"), nil},
		{stringptr("facade"), stringptr("api-facade"), nil},
		{stringptr("method"), stringptr("api-method"), nil},
		{stringptr("version"), stringptr("42"), nil},
	}

	metricFamilies, err := s.registry.Gather()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricFamilies, gc.HasLen, 2)
	c.Assert(metricFamilies, jc.DeepEquals, []*dto.MetricFamily{{
		Name: stringptr("juju_api_request_duration_seconds"),
		Help: stringptr("Latency of Juju API requests in seconds."),
		Type: metricTypePtr(dto.MetricType_SUMMARY),
		Metric: []*dto.Metric{{
			Label: labels,
			Summary: &dto.Summary{
				SampleCount: uint64ptr(3),
				SampleSum:   float64ptr(4.5),
				Quantile: []*dto.Quantile{{
					Quantile: float64ptr(0.5),
					Value:    float64ptr(1),
				}, {
					Quantile: float64ptr(0.9),
					Value:    float64ptr(1.5),
				}, {
					Quantile: float64ptr(0.99),
					Value:    float64ptr(1.5),
				}},
			},
		}},
	}, {
		Name: stringptr("juju_api_requests_total"),
		Help: stringptr("Number of Juju API requests served."),
		Type: metricTypePtr(dto.MetricType_COUNTER),
		Metric: []*dto.Metric{{
			Label: labels,
			Counter: &dto.Counter{
				Value: float64ptr(3),
			},
		}},
	}})
}
