// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/metricsender"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type MetricSenderSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&MetricSenderSuite{})

// TestSendMetrics creates 2 unsent metrics and a sent metric
// and checks that the 2 unsent metrics get sent and have their
// sent field set to true.
func (s *MetricSenderSuite) TestSendMetrics(c *gc.C) {
	var sender metricsender.MockSender
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now()
	unsent1 := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Time: &now})
	unsent2 := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Time: &now})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &now})
	err := metricsender.SendMetrics(s.State, &sender, 10)
	c.Assert(err, gc.IsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 2)

	sent1, err := s.State.MetricBatch(unsent1.UUID())
	c.Assert(err, gc.IsNil)
	c.Assert(sent1.Sent(), jc.IsTrue)

	sent2, err := s.State.MetricBatch(unsent2.UUID())
	c.Assert(err, gc.IsNil)
	c.Assert(sent2.Sent(), jc.IsTrue)
}

// TestSendBulkMetrics tests the logic of splitting sends
// into batches is done correctly. The batch size is changed
// to send batches of 10 metrics. If we create 100 metrics 10 calls
// will be made to the sender
func (s *MetricSenderSuite) TestSendBulkMetrics(c *gc.C) {
	var sender metricsender.MockSender
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now()
	for i := 0; i < 100; i++ {
		s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Time: &now})
	}
	err := metricsender.SendMetrics(s.State, &sender, 10)
	c.Assert(err, gc.IsNil)

	c.Assert(sender.Data, gc.HasLen, 10)
	for i := 0; i < 10; i++ {
		c.Assert(sender.Data, gc.HasLen, 10)
	}
}

// TestDontSendWithNopSender check that if the default sender
// is nil we don't send anything, but still mark the items as sent
func (s *MetricSenderSuite) TestDontSendWithNopSender(c *gc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now()
	for i := 0; i < 3; i++ {
		s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	}
	err := metricsender.SendMetrics(s.State, metricsender.NopSender{}, 10)
	c.Assert(err, gc.IsNil)
	sent, err := s.State.CountofSentMetrics()
	c.Assert(err, gc.IsNil)
	c.Assert(sent, gc.Equals, 3)
}

func (s *MetricSenderSuite) TestToWire(c *gc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now().Round(time.Second).UTC()
	metric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	result := metricsender.ToWire(metric)
	m := metric.Metrics()[0]
	metrics := []metricsender.Metric{
		{
			Key:         m.Key,
			Value:       m.Value,
			Time:        m.Time,
			Credentials: m.Credentials,
		},
	}
	expected := &metricsender.MetricBatch{
		UUID:     metric.UUID(),
		EnvUUID:  metric.EnvUUID(),
		Unit:     metric.Unit(),
		CharmUrl: metric.CharmURL(),
		Created:  metric.Created(),
		Metrics:  metrics,
	}
	c.Assert(result, gc.DeepEquals, expected)
}
