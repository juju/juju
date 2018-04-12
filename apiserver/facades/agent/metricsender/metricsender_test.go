// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender_test

import (
	"errors"
	"time"

	wireformat "github.com/juju/romulus/wireformat/metrics"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/metricsender"
	"github.com/juju/juju/apiserver/facades/agent/metricsender/testing"
	jujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type MetricSenderSuite struct {
	jujujutesting.JujuConnSuite
	meteredUnit *state.Unit
	credUnit    *state.Unit
	clock       clock.Clock
}

// TODO(externalreality): This may go away once the separation of
// responsibilities between state.State and the different model types become
// visible in the code.
type TestSenderBackend struct {
	*state.State
	*state.Model
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
	s.clock = jujutesting.NewClock(time.Now())
}

func (s *MetricSenderSuite) TestToWire(c *gc.C) {
	now := time.Now().Round(time.Second)
	metric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: false, Time: &now})
	result := metricsender.ToWire(metric, s.Model.Name())
	m := metric.Metrics()[0]
	metrics := []wireformat.Metric{
		{
			Key:   m.Key,
			Value: m.Value,
			Time:  m.Time.UTC(),
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}
	expected := &wireformat.MetricBatch{
		UUID:           metric.UUID(),
		ModelUUID:      metric.ModelUUID(),
		ModelName:      "controller",
		UnitName:       metric.Unit(),
		CharmUrl:       metric.CharmURL(),
		Created:        metric.Created().UTC(),
		Metrics:        metrics,
		Credentials:    metric.Credentials(),
		SLACredentials: metric.SLACredentials(),
	}
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *MetricSenderSuite) TestSendMetricsFromNewModel(c *gc.C) {
	var sender testing.MockSender
	now := time.Now()
	clock := jujutesting.NewClock(time.Now())

	state := s.Factory.MakeModel(c, &factory.ModelParams{Name: "test-model"})
	defer state.Close()
	f := factory.NewFactory(state)
	model, err := state.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelName := model.Name()
	c.Assert(modelName, gc.Equals, "test-model")

	meteredCharm := f.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	// Application with metrics credentials set.
	credApp := f.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm, Name: "cred"})
	err = credApp.SetMetricCredentials([]byte("something here"))
	c.Assert(err, jc.ErrorIsNil)
	meteredApp := f.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	meteredUnit := f.MakeUnit(c, &factory.UnitParams{Application: meteredApp, SetCharmURL: true})
	credUnit := f.MakeUnit(c, &factory.UnitParams{Application: credApp, SetCharmURL: true})

	f.MakeMetric(c, &factory.MetricParams{Unit: credUnit, Time: &now})
	f.MakeMetric(c, &factory.MetricParams{Unit: meteredUnit, Time: &now})
	f.MakeMetric(c, &factory.MetricParams{Unit: credUnit, Sent: true, Time: &now})
	err = metricsender.SendMetrics(TestSenderBackend{state, model}, &sender, clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 2)
	c.Assert(sender.Data[0][0].ModelName, gc.Equals, "test-model")
	c.Assert(sender.Data[0][1].ModelName, gc.Equals, "test-model")
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
	err := metricsender.SendMetrics(TestSenderBackend{s.State, s.IAASModel.Model}, &sender, s.clock, 10, true)
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

func (s *MetricSenderSuite) TestSendingHandlesModelMeterStatus(c *gc.C) {
	var sender testing.MockSender
	sender.MeterStatusResponse = "RED"
	now := time.Now()
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.meteredUnit, Time: &now})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: true, Time: &now})
	err := metricsender.SendMetrics(TestSenderBackend{s.State, s.IAASModel.Model}, &sender, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 2)

	meterStatus, err := s.State.ModelMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meterStatus.Code.String(), gc.Equals, "RED")
	c.Assert(meterStatus.Info, gc.Equals, "mocked response")
}

func (s *MetricSenderSuite) TestSendingHandlesEmptyModelMeterStatus(c *gc.C) {
	var sender testing.MockSender
	sender.MeterStatusResponse = ""
	now := time.Now()
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.meteredUnit, Time: &now})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: true, Time: &now})
	err := metricsender.SendMetrics(TestSenderBackend{s.State, s.IAASModel.Model}, &sender, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 2)

	meterStatus, err := s.State.ModelMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meterStatus.Code.String(), gc.Equals, "NOT AVAILABLE")
	c.Assert(meterStatus.Info, gc.Equals, "")
}

// TestSendMetricsAbort creates 7 unsent metrics and
// checks that the sending stops when no more batches are ack'ed.
func (s *MetricSenderSuite) TestSendMetricsAbort(c *gc.C) {
	sender := &testing.MockSender{}
	now := time.Now()
	metrics := make([]*state.MetricBatch, 7)
	for i := 0; i < 7; i++ {
		metrics[i] = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	}

	sender.IgnoreBatches(metrics[0:2]...)

	// Send 4 batches per POST.
	err := metricsender.SendMetrics(TestSenderBackend{s.State, s.IAASModel.Model}, sender, s.clock, 4, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 4)

	unsent := 0
	sent := 0
	for _, batch := range metrics {
		b, err := s.State.MetricBatch(batch.UUID())
		c.Assert(err, jc.ErrorIsNil)
		if b.Sent() {
			sent++
		} else {
			unsent++
		}
	}
	c.Assert(sent, gc.Equals, 5)
	c.Assert(unsent, gc.Equals, 2)
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
	err := metricsender.SendMetrics(TestSenderBackend{s.State, s.IAASModel.Model}, &sender, s.clock, 10, false)
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
	err = metricsender.SendMetrics(TestSenderBackend{s.State, s.IAASModel.Model}, &sender, s.clock, 10, false)
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
	err := metricsender.SendMetrics(TestSenderBackend{s.State, s.IAASModel.Model}, &sender, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(sender.Data, gc.HasLen, 10)
	for _, d := range sender.Data {
		c.Assert(d, gc.HasLen, 10)
	}
}

// TestDontSendWithNopSender check that if the default sender
// is nil we don't send anything, but still mark the items as sent
func (s *MetricSenderSuite) TestDontSendWithNopSender(c *gc.C) {
	now := time.Now()
	for i := 0; i < 3; i++ {
		s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: false, Time: &now})
	}
	err := metricsender.SendMetrics(TestSenderBackend{s.State, s.IAASModel.Model}, metricsender.NopSender{}, s.clock, 10, true)
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
	err := metricsender.SendMetrics(TestSenderBackend{s.State, s.IAASModel.Model}, sender, s.clock, 1, true)
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
	err = metricsender.SendMetrics(TestSenderBackend{s.State, s.IAASModel.Model}, metricsender.NopSender{}, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	mm, err = s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.ConsecutiveErrors(), gc.Equals, 0)
}
