// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/metricsender"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type MetricSuite struct {
	ConnSuite
	unit *state.Unit
}

var _ = gc.Suite(&MetricSuite{})

func (s *MetricSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.unit = s.assertAddUnit(c)
}

func (s *MetricSuite) TestAddNoMetrics(c *gc.C) {
	now := state.NowToTheSecond()
	_, err := s.unit.AddMetrics(now, []state.Metric{})
	c.Assert(err, gc.ErrorMatches, "cannot add a batch of 0 metrics")
}

func (s *MetricSuite) TestAddMetric(c *gc.C) {
	now := state.NowToTheSecond()
	environTag := s.State.EnvironTag().String()
	m := state.Metric{"item", "5", now, []byte("creds")}
	metricBatch, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, gc.IsNil)
	c.Assert(metricBatch.Unit(), gc.Equals, "wordpress/0")
	c.Assert(metricBatch.EnvUUID(), gc.Equals, environTag)
	c.Assert(metricBatch.CharmURL(), gc.Equals, "local:quantal/quantal-wordpress-3")
	c.Assert(metricBatch.Sent(), gc.Equals, false)
	c.Assert(metricBatch.Metrics(), gc.HasLen, 1)

	metric := metricBatch.Metrics()[0]
	c.Assert(metric.Key, gc.Equals, "item")
	c.Assert(metric.Value, gc.Equals, "5")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	c.Assert(metric.Credentials, gc.DeepEquals, []byte("creds"))

	saved, err := s.State.MetricBatch(metricBatch.UUID())
	c.Assert(err, gc.IsNil)
	c.Assert(saved.Unit(), gc.Equals, "wordpress/0")
	c.Assert(metricBatch.CharmURL(), gc.Equals, "local:quantal/quantal-wordpress-3")
	c.Assert(saved.Sent(), gc.Equals, false)
	c.Assert(saved.Metrics(), gc.HasLen, 1)
	metric = saved.Metrics()[0]
	c.Assert(metric.Key, gc.Equals, "item")
	c.Assert(metric.Value, gc.Equals, "5")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	c.Assert(metric.Credentials, gc.DeepEquals, []byte("creds"))
}

func assertUnitRemoved(c *gc.C, unit *state.Unit) {
	assertUnitDead(c, unit)
	err := unit.Remove()
	c.Assert(err, gc.IsNil)
}

func assertUnitDead(c *gc.C, unit *state.Unit) {
	err := unit.EnsureDead()
	c.Assert(err, gc.IsNil)
}

func (s *MetricSuite) assertAddUnit(c *gc.C) *state.Unit {
	charm := s.AddTestingCharm(c, "wordpress")
	svc := s.AddTestingService(c, "wordpress", charm)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.SetCharmURL(charm.URL())
	c.Assert(err, gc.IsNil)
	return unit
}

func (s *MetricSuite) TestAddMetricNonExitentUnit(c *gc.C) {
	assertUnitRemoved(c, s.unit)
	now := state.NowToTheSecond()
	m := state.Metric{"item", "5", now, []byte{}}
	_, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, gc.ErrorMatches, `wordpress/0 not found`)
}

func (s *MetricSuite) TestAddMetricDeadUnit(c *gc.C) {
	assertUnitDead(c, s.unit)
	now := state.NowToTheSecond()
	m := state.Metric{"item", "5", now, []byte{}}
	_, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, gc.ErrorMatches, `wordpress/0 not found`)
}

func (s *MetricSuite) TestSetMetricSent(c *gc.C) {
	now := state.NowToTheSecond()
	m := state.Metric{"item", "5", now, []byte{}}
	added, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, gc.IsNil)
	saved, err := s.State.MetricBatch(added.UUID())
	c.Assert(err, gc.IsNil)
	err = saved.SetSent()
	c.Assert(err, gc.IsNil)
	c.Assert(saved.Sent(), jc.IsTrue)
	saved, err = s.State.MetricBatch(added.UUID())
	c.Assert(err, gc.IsNil)
	c.Assert(saved.Sent(), jc.IsTrue)
}

func (s *MetricSuite) TestCleanupMetrics(c *gc.C) {
	oldTime := time.Now().Add(-(time.Hour * 25))
	m := state.Metric{"item", "5", oldTime, []byte("creds")}
	oldMetric, err := s.unit.AddMetrics(oldTime, []state.Metric{m})
	c.Assert(err, gc.IsNil)
	oldMetric.SetSent()

	now := time.Now()
	m = state.Metric{"item", "5", now, []byte("creds")}
	newMetric, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, gc.IsNil)
	newMetric.SetSent()
	err = s.State.CleanupOldMetrics()
	c.Assert(err, gc.IsNil)

	_, err = s.State.MetricBatch(newMetric.UUID())
	c.Assert(err, gc.IsNil)

	_, err = s.State.MetricBatch(oldMetric.UUID())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *MetricSuite) TestCleanupNoMetrics(c *gc.C) {
	err := s.State.CleanupOldMetrics()
	c.Assert(err, gc.IsNil)
}

func (s *MetricSuite) TestMetricBatches(c *gc.C) {
	now := state.NowToTheSecond()
	m := state.Metric{"item", "5", now, []byte("creds")}
	_, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, gc.IsNil)
	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, gc.IsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	c.Assert(metricBatches[0].Unit(), gc.Equals, "wordpress/0")
	c.Assert(metricBatches[0].CharmURL(), gc.Equals, "local:quantal/quantal-wordpress-3")
	c.Assert(metricBatches[0].Sent(), gc.Equals, false)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
}

// TestSendMetrics creates 2 unsent metrics and a sent metric
// and checks that the 2 unsent metrics get sent and have their
// sent field set to true.
func (s *MetricSuite) TestSendMetrics(c *gc.C) {
	unit := s.factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now()
	unsent1 := s.factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Time: &now})
	unsent2 := s.factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Time: &now})
	s.factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &now})
	sender := &testing.MockSender{}
	err := s.State.SendMetrics(sender, 10)
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
func (s *MetricSuite) TestSendBulkMetrics(c *gc.C) {
	sender := &testing.MockSender{}
	unit := s.factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now()
	for i := 0; i < 100; i++ {
		s.factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Time: &now})
	}
	err := s.State.SendMetrics(sender, 10)
	c.Assert(err, gc.IsNil)

	c.Assert(sender.Data, gc.HasLen, 10)
	for i := 0; i < 10; i++ {
		c.Assert(sender.Data, gc.HasLen, 10)
	}
}

// TestCountMetrics asserts the correct values are returned
// by countofUnsentMetrics and countofSentMetrics.
func (s *MetricSuite) TestCountMetrics(c *gc.C) {
	unit := s.factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now()
	s.factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	s.factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	s.factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &now})
	sent, err := state.CountofSentMetrics(s.State)
	c.Assert(err, gc.IsNil)
	c.Assert(sent, gc.Equals, 1)
	unsent, err := state.CountofUnsentMetrics(s.State)
	c.Assert(err, gc.IsNil)
	c.Assert(unsent, gc.Equals, 2)
	c.Assert(unsent+sent, gc.Equals, 3)
}

// TestDontSendWithNopSender check that if the default sender
// is nil we don't send anything, but still mark the items as sent
func (s *MetricSuite) TestDontSendWithNilSender(c *gc.C) {
	unit := s.factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now()
	for i := 0; i < 3; i++ {
		s.factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	}
	err := s.State.SendMetrics(&metricsender.NopSender{}, 10)
	c.Assert(err, gc.IsNil)
	sent, err := state.CountofSentMetrics(s.State)
	c.Assert(err, gc.IsNil)
	c.Assert(sent, gc.Equals, 3)

}

func (s *MetricSuite) TestSetMetricBatchesSent(c *gc.C) {
	unit := s.factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now()
	metrics := make([]*state.MetricBatch, 3)
	for i, _ := range metrics {
		metrics[i] = s.factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	}
	err := state.SetMetricBatchesSent(s.State, metrics)
	c.Assert(err, gc.IsNil)
	sent, err := state.CountofSentMetrics(s.State)
	c.Assert(err, gc.IsNil)
	c.Assert(sent, gc.Equals, 3)

}
