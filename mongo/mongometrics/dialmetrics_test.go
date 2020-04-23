// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package mongometrics_test

import (
	"errors"
	"net"
	"reflect"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo/mongometrics"
)

type DialCollectorSuite struct {
	testing.IsolationSuite
	collector *mongometrics.DialCollector
}

var _ = gc.Suite(&DialCollectorSuite{})

func (s *DialCollectorSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.collector = mongometrics.NewDialCollector()
}

func (s *DialCollectorSuite) TestDescribe(c *gc.C) {
	ch := make(chan *prometheus.Desc)
	go func() {
		defer close(ch)
		s.collector.Describe(ch)
	}()
	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}
	c.Assert(descs, gc.HasLen, 2)
	c.Assert(descs[0].String(), gc.Matches, `.*fqName: "juju_mongo_dials_total".*`)
	c.Assert(descs[1].String(), gc.Matches, `.*fqName: "juju_mongo_dial_duration_seconds".*`)
}

func (s *DialCollectorSuite) TestCollect(c *gc.C) {
	s.collector.PostDialServer("foo", time.Second, nil)
	s.collector.PostDialServer("foo", 2*time.Second, nil)
	s.collector.PostDialServer("foo", 3*time.Second, nil)
	s.collector.PostDialServer("bar", time.Millisecond, errors.New("bewm"))
	s.collector.PostDialServer("baz", time.Minute, &net.OpError{
		Op:  "read",
		Err: errors.New("bewm"),
	})
	s.collector.PostDialServer("qux", time.Hour, netError{
		error:     errors.New("bewm"),
		timeout:   true,
		temporary: true,
	})

	ch := make(chan prometheus.Metric)
	go func() {
		defer close(ch)
		s.collector.Collect(ch)
	}()

	var dtoMetrics [8]dto.Metric
	var metrics []prometheus.Metric
	for metric := range ch {
		metrics = append(metrics, metric)
	}
	c.Assert(metrics, gc.HasLen, len(dtoMetrics))

	for i, metric := range metrics {
		err := metric.Write(&dtoMetrics[i])
		c.Assert(err, jc.ErrorIsNil)
	}

	float64ptr := func(v float64) *float64 {
		return &v
	}
	uint64ptr := func(v uint64) *uint64 {
		return &v
	}
	labelpair := func(n, v string) *dto.LabelPair {
		return &dto.LabelPair{Name: &n, Value: &v}
	}
	expected := []dto.Metric{{
		Counter: &dto.Counter{Value: float64ptr(3)},
		Label: []*dto.LabelPair{
			labelpair("failed", ""),
			labelpair("server", "foo"),
			labelpair("timeout", ""),
		},
	}, {
		Counter: &dto.Counter{Value: float64ptr(1)},
		Label: []*dto.LabelPair{
			labelpair("failed", "failed"),
			labelpair("server", "bar"),
			labelpair("timeout", ""),
		},
	}, {
		Counter: &dto.Counter{Value: float64ptr(1)},
		Label: []*dto.LabelPair{
			labelpair("failed", "read"),
			labelpair("server", "baz"),
			labelpair("timeout", ""),
		},
	}, {
		Counter: &dto.Counter{Value: float64ptr(1)},
		Label: []*dto.LabelPair{
			labelpair("failed", "failed"),
			labelpair("server", "qux"),
			labelpair("timeout", "timed out"),
		},
	}, {
		Label: []*dto.LabelPair{
			labelpair("failed", ""),
			labelpair("server", "foo"),
			labelpair("timeout", ""),
		},
		Summary: &dto.Summary{
			SampleCount: uint64ptr(3),
			SampleSum:   float64ptr(6),
			Quantile: []*dto.Quantile{{
				Quantile: float64ptr(0.5),
				Value:    float64ptr(2),
			}, {
				Quantile: float64ptr(0.9),
				Value:    float64ptr(3),
			}, {
				Quantile: float64ptr(0.99),
				Value:    float64ptr(3),
			}},
		},
	}, {
		Label: []*dto.LabelPair{
			labelpair("failed", "failed"),
			labelpair("server", "bar"),
			labelpair("timeout", ""),
		},
		Summary: &dto.Summary{
			SampleCount: uint64ptr(1),
			SampleSum:   float64ptr(0.001),
			Quantile: []*dto.Quantile{{
				Quantile: float64ptr(0.5),
				Value:    float64ptr(0.001),
			}, {
				Quantile: float64ptr(0.9),
				Value:    float64ptr(0.001),
			}, {
				Quantile: float64ptr(0.99),
				Value:    float64ptr(0.001),
			}},
		},
	}, {
		Label: []*dto.LabelPair{
			labelpair("failed", "read"),
			labelpair("server", "baz"),
			labelpair("timeout", ""),
		},
		Summary: &dto.Summary{
			SampleCount: uint64ptr(1),
			SampleSum:   float64ptr(60),
			Quantile: []*dto.Quantile{{
				Quantile: float64ptr(0.5),
				Value:    float64ptr(60),
			}, {
				Quantile: float64ptr(0.9),
				Value:    float64ptr(60),
			}, {
				Quantile: float64ptr(0.99),
				Value:    float64ptr(60),
			}},
		},
	}, {
		Label: []*dto.LabelPair{
			labelpair("failed", "failed"),
			labelpair("server", "qux"),
			labelpair("timeout", "timed out"),
		},
		Summary: &dto.Summary{
			SampleCount: uint64ptr(1),
			SampleSum:   float64ptr(3600),
			Quantile: []*dto.Quantile{{
				Quantile: float64ptr(0.5),
				Value:    float64ptr(3600),
			}, {
				Quantile: float64ptr(0.9),
				Value:    float64ptr(3600),
			}, {
				Quantile: float64ptr(0.99),
				Value:    float64ptr(3600),
			}},
		},
	}}
	for _, dm := range dtoMetrics {
		var found bool
		for i, m := range expected {
			if !reflect.DeepEqual(dm, m) {
				continue
			}
			expected = append(expected[:i], expected[i+1:]...)
			found = true
			break
		}
		if !found {
			c.Errorf("metric %+v not expected", dm)
		}
	}
}

type netError struct {
	error
	timeout   bool
	temporary bool
}

func (e netError) Timeout() bool {
	return e.timeout
}

func (e netError) Temporary() bool {
	return e.temporary
}
