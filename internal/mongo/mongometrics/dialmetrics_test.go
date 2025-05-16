// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package mongometrics_test

import (
	"errors"
	"net"
	"reflect"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/juju/juju/internal/mongo/mongometrics"
	"github.com/juju/juju/internal/testhelpers"
)

type DialCollectorSuite struct {
	testhelpers.IsolationSuite
	collector *mongometrics.DialCollector
}

func TestDialCollectorSuite(t *stdtesting.T) { tc.Run(t, &DialCollectorSuite{}) }
func (s *DialCollectorSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.collector = mongometrics.NewDialCollector()
}

func (s *DialCollectorSuite) TestDescribe(c *tc.C) {
	ch := make(chan *prometheus.Desc)
	go func() {
		defer close(ch)
		s.collector.Describe(ch)
	}()
	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}
	c.Assert(descs, tc.HasLen, 2)
	c.Assert(descs[0].String(), tc.Matches, `.*fqName: "juju_mongo_dials_total".*`)
	c.Assert(descs[1].String(), tc.Matches, `.*fqName: "juju_mongo_dial_duration_seconds".*`)
}

func (s *DialCollectorSuite) TestCollect(c *tc.C) {
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

	var dtoMetrics [8]*dto.Metric
	var metrics []prometheus.Metric
	for metric := range ch {
		metrics = append(metrics, metric)
	}
	c.Assert(metrics, tc.HasLen, len(dtoMetrics))

	for i, metric := range metrics {
		dtoMetrics[i] = &dto.Metric{}
		err := metric.Write(dtoMetrics[i])
		c.Assert(err, tc.ErrorIsNil)
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
	expected := []*dto.Metric{{
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
		dm.TimestampMs = nil
		if dm.Counter != nil {
			dm.Counter.CreatedTimestamp = nil
		}
		if dm.Histogram != nil {
			dm.Histogram.CreatedTimestamp = nil
		}
		if dm.Summary != nil {
			dm.Summary.CreatedTimestamp = nil
		}
		var found bool
		for j := range expected {
			if !metricsEqual(dm, expected[j]) {
				continue
			}
			expected = append(expected[:j], expected[j+1:]...)
			found = true
			break
		}
		if !found {
			c.Errorf("metric %+v not expected", dm)
		}
	}
}

func metricsEqual(m1 *dto.Metric, m2 *dto.Metric) bool {
	if !reflect.DeepEqual(m1.Label, m2.Label) {
		return false
	}
	if !reflect.DeepEqual(m1.Gauge, m2.Gauge) {
		return false
	}
	if m1.Counter != nil || m2.Counter != nil {
		if m1.Counter == nil || m2.Counter == nil {
			return false
		}
		if m1.Counter.GetValue() != m2.Counter.GetValue() {
			return false
		}
	}
	if m1.Gauge != nil || m2.Gauge != nil {
		if m1.Gauge == nil || m2.Gauge == nil {
			return false
		}
		if m1.Gauge.GetValue() != m2.Gauge.GetValue() {
			return false
		}
	}
	if m1.Summary != nil || m2.Summary != nil {
		if m1.Summary == nil || m2.Summary == nil {
			return false
		}
		if m1.Summary.GetSampleSum() != m2.Summary.GetSampleSum() {
			return false
		}
		if m1.Summary.GetSampleCount() != m2.Summary.GetSampleCount() {
			return false
		}
		if !reflect.DeepEqual(m1.Summary.GetQuantile(), m2.Summary.GetQuantile()) {
			return false
		}
	}
	if m1.Histogram != nil || m2.Histogram != nil {
		if m1.Histogram == nil || m2.Histogram == nil {
			return false
		}
		if m1.Histogram.GetSampleSum() != m2.Histogram.GetSampleSum() {
			return false
		}
		if m1.Histogram.GetSampleCount() != m2.Histogram.GetSampleCount() {
			return false
		}
		if !reflect.DeepEqual(m1.Histogram.GetBucket(), m2.Histogram.GetBucket()) {
			return false
		}
	}

	return true
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
