// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender_test

import (
	"errors"
	"time"

	wireformat "github.com/juju/romulus/wireformat/metrics"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/metricsender"
	"github.com/juju/juju/apiserver/metricsender/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type MetricSenderSuite struct {
	jujutesting.JujuConnSuite
	meteredUnit *state.Unit
	credUnit    *state.Unit
}

var _ = gc.Suite(&MetricSenderSuite{})

var _ metricsender.MetricSender = (*testing.MockSender)(nil)

var _ metricsender.MetricSender = (*metricsender.NopSender)(nil)

func (s *MetricSenderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	// Application with metrics credentials set.
	credApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm, Name: "cred"})
	err := credApp.SetMetricCredentials([]byte("something here"))
	c.Assert(err, jc.ErrorIsNil)
	meteredApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	s.meteredUnit = s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredApp, SetCharmURL: true})
	s.credUnit = s.Factory.MakeUnit(c, &factory.UnitParams{Application: credApp, SetCharmURL: true})
}

func (s *MetricSenderSuite) TestToWire(c *gc.C) {
	now := time.Now().Round(time.Second)
	metric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: false, Time: &now})
	result := metricsender.ToWire(metric)
	m := metric.Metrics()[0]
	metrics := []wireformat.Metric{
		{
			Key:   m.Key,
			Value: m.Value,
			Time:  m.Time.UTC(),
		},
	}
	expected := &wireformat.MetricBatch{
		UUID:        metric.UUID(),
		ModelUUID:   metric.ModelUUID(),
		UnitName:    metric.Unit(),
		CharmUrl:    metric.CharmURL(),
		Created:     metric.Created().UTC(),
		Metrics:     metrics,
		Credentials: metric.Credentials(),
	}
	c.Assert(result, gc.DeepEquals, expected)
}

// TestSendMetrics creates 2 unsent metrics and a sent metric
// and checks that the 2 unsent metrics get marked as sent (have their
// sent field set to true).
func (s *MetricSenderSuite) TestSendMetrics(c *gc.C) {
	var sender testing.MockSender
	now := time.Now()
	unsent1 := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	unsent2 := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.meteredUnit, Time: &now})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: true, Time: &now})
	err := metricsender.SendMetrics(s.State, &sender, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 2)

	sent1, err := s.State.MetricBatch(unsent1.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent1.Sent(), jc.IsTrue)

	sent2, err := s.State.MetricBatch(unsent2.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent2.Sent(), jc.IsTrue)
}

// TestHoldMetrics creates 2 unsent metrics and a sent metric
// and checks that only the metric from the application with credentials is sent.
// But both metrics are marked as sent.
func (s *MetricSenderSuite) TestHoldMetrics(c *gc.C) {
	var sender testing.MockSender
	now := time.Now()
	unsent1 := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	unsent2 := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.meteredUnit, Time: &now})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: true, Time: &now})
	err := metricsender.SendMetrics(s.State, &sender, 10, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 1)
	c.Assert(sender.Data[0][0].UUID, gc.Equals, unsent1.UUID())
	sent1, err := s.State.MetricBatch(unsent1.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent1.Sent(), jc.IsTrue)

	sent2, err := s.State.MetricBatch(unsent2.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent2.Sent(), jc.IsTrue)
}

func (s *MetricSenderSuite) TestHoldMetricsSetsMeterStatus(c *gc.C) {
	var sender testing.MockSender
	now := time.Now()
	err := s.credUnit.SetMeterStatus("GREEN", "known starting point")
	c.Assert(err, jc.ErrorIsNil)
	err = s.meteredUnit.SetMeterStatus("GREEN", "known starting point")
	c.Assert(err, jc.ErrorIsNil)
	unsent1 := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.meteredUnit, Time: &now})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: true, Time: &now})
	err = metricsender.SendMetrics(s.State, &sender, 10, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 1)
	c.Assert(sender.Data[0][0].UUID, gc.Equals, unsent1.UUID())
	msCred, err := s.credUnit.GetMeterStatus()
	c.Assert(msCred.Code, gc.Equals, state.MeterGreen)
	c.Assert(msCred.Info, gc.Equals, "known starting point")
	msMetered, err := s.meteredUnit.GetMeterStatus()
	c.Assert(msMetered.Code, gc.Equals, state.MeterRed)
	c.Assert(msMetered.Info, gc.Equals, "transmit-vendor-metrics turned off")
}

// TestSendBulkMetrics tests the logic of splitting sends
// into batches is done correctly. The batch size is changed
// to send batches of 10 metrics. If we create 100 metrics 10 calls
// will be made to the sender
func (s *MetricSenderSuite) TestSendBulkMetrics(c *gc.C) {
	var sender testing.MockSender
	now := time.Now()
	for i := 0; i < 100; i++ {
		s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	}
	err := metricsender.SendMetrics(s.State, &sender, 10, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(sender.Data, gc.HasLen, 10)
	for i := 0; i < 10; i++ {
		c.Assert(sender.Data, gc.HasLen, 10)
	}
}

// TestDontSendWithNopSender check that if the default sender
// is nil we don't send anything, but still mark the items as sent
func (s *MetricSenderSuite) TestDontSendWithNopSender(c *gc.C) {
	now := time.Now()
	for i := 0; i < 3; i++ {
		s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: false, Time: &now})
	}
	err := metricsender.SendMetrics(s.State, metricsender.NopSender{}, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	sent, err := s.State.CountOfSentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent, gc.Equals, 3)
}

func (s *MetricSenderSuite) TestFailureIncrementsConsecutiveFailures(c *gc.C) {
	sender := &testing.ErrorSender{Err: errors.New("something went wrong")}
	now := time.Now()
	for i := 0; i < 3; i++ {
		s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: false, Time: &now})
	}
	err := metricsender.SendMetrics(s.State, sender, 1, true)
	c.Assert(err, gc.ErrorMatches, "something went wrong")
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.ConsecutiveErrors(), gc.Equals, 1)
}

func (s *MetricSenderSuite) TestFailuresResetOnSuccessfulSend(c *gc.C) {
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	err = mm.IncrementConsecutiveErrors()
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	for i := 0; i < 3; i++ {
		s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: false, Time: &now})
	}
	err = metricsender.SendMetrics(s.State, metricsender.NopSender{}, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	mm, err = s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.ConsecutiveErrors(), gc.Equals, 0)
}
