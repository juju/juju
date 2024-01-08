// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender_test

import (
	"context"
	"errors"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	wireformat "github.com/juju/romulus/wireformat/metrics"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/metricsender"
	"github.com/juju/juju/apiserver/facades/agent/metricsender/testing"
	jujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type MetricSenderSuite struct {
	jujujutesting.ApiServerSuite
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
	s.ApiServerSuite.SetUpTest(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	meteredCharm := f.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "ch:quantal/metered"})
	// Application with metrics credentials set.
	credApp := f.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm, Name: "cred"})
	err := credApp.SetMetricCredentials([]byte("something here"))
	c.Assert(err, jc.ErrorIsNil)
	meteredApp := f.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	s.meteredUnit = f.MakeUnit(c, &factory.UnitParams{Application: meteredApp, SetCharmURL: true})
	s.credUnit = f.MakeUnit(c, &factory.UnitParams{Application: credApp, SetCharmURL: true})
	s.clock = testclock.NewClock(time.Now())
}

func (s *MetricSenderSuite) TestToWire(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	now := time.Now().Round(time.Second)
	metric := f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: false, Time: &now})
	result := metricsender.ToWire(metric, s.ControllerModel(c).Name())
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
	clock := testclock.NewClock(time.Now())

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	st := f.MakeModel(c, &factory.ModelParams{Name: "test-model"})
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelName := model.Name()
	c.Assert(modelName, gc.Equals, "test-model")

	f2, release := s.NewFactory(c, model.UUID())
	defer release()

	meteredCharm := f2.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "ch:quantal/metered"})
	// Application with metrics credentials set.
	credApp := f2.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm, Name: "cred"})
	err = credApp.SetMetricCredentials([]byte("something here"))
	c.Assert(err, jc.ErrorIsNil)
	meteredApp := f2.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	meteredUnit := f2.MakeUnit(c, &factory.UnitParams{Application: meteredApp, SetCharmURL: true})
	credUnit := f2.MakeUnit(c, &factory.UnitParams{Application: credApp, SetCharmURL: true})

	f2.MakeMetric(c, &factory.MetricParams{Unit: credUnit, Time: &now})
	f2.MakeMetric(c, &factory.MetricParams{Unit: meteredUnit, Time: &now})
	f2.MakeMetric(c, &factory.MetricParams{Unit: credUnit, Sent: true, Time: &now})
	err = metricsender.SendMetrics(context.Background(), TestSenderBackend{st, model}, &sender, clock, 10, true)
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

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	unsent1 := f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	unsent2 := f.MakeMetric(c, &factory.MetricParams{Unit: s.meteredUnit, Time: &now})
	f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: true, Time: &now})

	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, &sender, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 2)

	sent1, err := st.MetricBatch(unsent1.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent1.Sent(), jc.IsTrue)

	sent2, err := st.MetricBatch(unsent2.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent2.Sent(), jc.IsTrue)
}

func (s *MetricSenderSuite) TestSendingHandlesModelMeterStatus(c *gc.C) {
	var sender testing.MockSender
	sender.MeterStatusResponse = "RED"
	now := time.Now()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	f.MakeMetric(c, &factory.MetricParams{Unit: s.meteredUnit, Time: &now})
	f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: true, Time: &now})

	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, &sender, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 2)

	meterStatus, err := st.ModelMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meterStatus.Code.String(), gc.Equals, "RED")
	c.Assert(meterStatus.Info, gc.Equals, "mocked response")
}

func (s *MetricSenderSuite) TestSendingHandlesEmptyModelMeterStatus(c *gc.C) {
	var sender testing.MockSender
	sender.MeterStatusResponse = ""
	now := time.Now()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	f.MakeMetric(c, &factory.MetricParams{Unit: s.meteredUnit, Time: &now})
	f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: true, Time: &now})

	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, &sender, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 2)

	meterStatus, err := st.ModelMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meterStatus.Code.String(), gc.Equals, "NOT AVAILABLE")
	c.Assert(meterStatus.Info, gc.Equals, "")
}

// TestSendMetricsAbort creates 7 unsent metrics and
// checks that the sending stops when no more batches are ack'ed.
func (s *MetricSenderSuite) TestSendMetricsAbort(c *gc.C) {
	sender := &testing.MockSender{}
	now := time.Now()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	metrics := make([]*state.MetricBatch, 7)
	for i := 0; i < 7; i++ {
		metrics[i] = f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	}

	sender.IgnoreBatches(metrics[0:2]...)

	// Send 4 batches per POST.
	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, sender, s.clock, 4, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 4)

	unsent := 0
	sent := 0
	for _, batch := range metrics {
		b, err := st.MetricBatch(batch.UUID())
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

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	unsent1 := f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	unsent2 := f.MakeMetric(c, &factory.MetricParams{Unit: s.meteredUnit, Time: &now})
	f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: true, Time: &now})

	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, &sender, s.clock, 10, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 1)
	c.Assert(sender.Data[0][0].UUID, gc.Equals, unsent1.UUID())
	sent1, err := st.MetricBatch(unsent1.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent1.Sent(), jc.IsTrue)

	sent2, err := st.MetricBatch(unsent2.UUID())
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

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	unsent1 := f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	f.MakeMetric(c, &factory.MetricParams{Unit: s.meteredUnit, Time: &now})
	f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: true, Time: &now})

	st := s.ControllerModel(c).State()
	err = metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, &sender, s.clock, 10, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 1)
	c.Assert(sender.Data[0][0].UUID, gc.Equals, unsent1.UUID())
	msCred, err := s.credUnit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(msCred.Code, gc.Equals, state.MeterGreen)
	c.Assert(msCred.Info, gc.Equals, "known starting point")
	msMetered, err := s.meteredUnit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
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

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	for i := 0; i < 100; i++ {
		f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Time: &now})
	}

	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, &sender, s.clock, 10, true)
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

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	for i := 0; i < 3; i++ {
		f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: false, Time: &now})
	}

	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, metricsender.NopSender{}, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	sent, err := st.CountOfSentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent, gc.Equals, 3)
}

func (s *MetricSenderSuite) TestFailureIncrementsConsecutiveFailures(c *gc.C) {
	sender := &testing.ErrorSender{Err: errors.New("something went wrong")}
	now := time.Now()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	for i := 0; i < 3; i++ {
		f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: false, Time: &now})
	}

	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, sender, s.clock, 1, true)
	c.Assert(err, gc.ErrorMatches, "something went wrong")
	mm, err := st.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.ConsecutiveErrors(), gc.Equals, 1)
}

func (s *MetricSenderSuite) TestFailuresResetOnSuccessfulSend(c *gc.C) {
	st := s.ControllerModel(c).State()
	mm, err := st.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	err = mm.IncrementConsecutiveErrors()
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	for i := 0; i < 3; i++ {
		f.MakeMetric(c, &factory.MetricParams{Unit: s.credUnit, Sent: false, Time: &now})
	}
	err = metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, metricsender.NopSender{}, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	mm, err = st.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.ConsecutiveErrors(), gc.Equals, 0)
}
