// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type MetricSuite struct {
	ConnSuite
	unit         *state.Unit
	meteredCharm *state.Charm
}

var _ = gc.Suite(&MetricSuite{})

func (s *MetricSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.assertAddUnit(c)
}

func (s *MetricSuite) TestAddNoMetrics(c *gc.C) {
	now := state.NowToTheSecond()
	_, err := s.unit.AddMetrics(now, []state.Metric{})
	c.Assert(err, gc.ErrorMatches, "cannot add a batch of 0 metrics")
}

func (s *MetricSuite) TestAddMetric(c *gc.C) {
	now := state.NowToTheSecond()
	envUUID := s.State.EnvironUUID()
	m := state.Metric{"pings", "5", now, []byte("creds")}
	metricBatch, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatch.Unit(), gc.Equals, "metered/0")
	c.Assert(metricBatch.EnvUUID(), gc.Equals, envUUID)
	c.Assert(metricBatch.CharmURL(), gc.Equals, "cs:quantal/metered")
	c.Assert(metricBatch.Sent(), jc.IsFalse)
	c.Assert(metricBatch.Created(), gc.Equals, now)
	c.Assert(metricBatch.Metrics(), gc.HasLen, 1)

	metric := metricBatch.Metrics()[0]
	c.Assert(metric.Key, gc.Equals, "pings")
	c.Assert(metric.Value, gc.Equals, "5")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	c.Assert(metric.Credentials, gc.DeepEquals, []byte("creds"))

	saved, err := s.State.MetricBatch(metricBatch.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Unit(), gc.Equals, "metered/0")
	c.Assert(metricBatch.CharmURL(), gc.Equals, "cs:quantal/metered")
	c.Assert(saved.Sent(), jc.IsFalse)
	c.Assert(saved.Metrics(), gc.HasLen, 1)
	metric = saved.Metrics()[0]
	c.Assert(metric.Key, gc.Equals, "pings")
	c.Assert(metric.Value, gc.Equals, "5")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	c.Assert(metric.Credentials, gc.DeepEquals, []byte("creds"))
}

func assertUnitRemoved(c *gc.C, unit *state.Unit) {
	assertUnitDead(c, unit)
	err := unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func assertUnitDead(c *gc.C, unit *state.Unit) {
	err := unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricSuite) assertAddUnit(c *gc.C) {
	s.meteredCharm = s.factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.factory.MakeService(c, &factory.ServiceParams{Charm: s.meteredCharm})
	s.unit = s.factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
}

func (s *MetricSuite) TestAddMetricNonExistentUnit(c *gc.C) {
	assertUnitRemoved(c, s.unit)
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now, []byte{}}
	_, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, gc.ErrorMatches, `metered/0 not found`)
}

func (s *MetricSuite) TestAddMetricDeadUnit(c *gc.C) {
	assertUnitDead(c, s.unit)
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now, []byte{}}
	_, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, gc.ErrorMatches, `metered/0 not found`)
}

func (s *MetricSuite) TestSetMetricSent(c *gc.C) {
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now, []byte{}}
	added, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, jc.ErrorIsNil)
	saved, err := s.State.MetricBatch(added.UUID())
	c.Assert(err, jc.ErrorIsNil)
	err = saved.SetSent()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Sent(), jc.IsTrue)
	saved, err = s.State.MetricBatch(added.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Sent(), jc.IsTrue)
}

func (s *MetricSuite) TestCleanupMetrics(c *gc.C) {
	oldTime := time.Now().Add(-(time.Hour * 25))
	m := state.Metric{"pings", "5", oldTime, []byte("creds")}
	oldMetric, err := s.unit.AddMetrics(oldTime, []state.Metric{m})
	c.Assert(err, jc.ErrorIsNil)
	oldMetric.SetSent()

	now := time.Now()
	m = state.Metric{"pings", "5", now, []byte("creds")}
	newMetric, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, jc.ErrorIsNil)
	newMetric.SetSent()
	err = s.State.CleanupOldMetrics()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.MetricBatch(newMetric.UUID())
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.MetricBatch(oldMetric.UUID())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *MetricSuite) TestCleanupNoMetrics(c *gc.C) {
	err := s.State.CleanupOldMetrics()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricSuite) TestMetricBatches(c *gc.C) {
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now, []byte("creds")}
	_, err := s.unit.AddMetrics(now, []state.Metric{m})
	c.Assert(err, jc.ErrorIsNil)
	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	c.Assert(metricBatches[0].Unit(), gc.Equals, "metered/0")
	c.Assert(metricBatches[0].CharmURL(), gc.Equals, "cs:quantal/metered")
	c.Assert(metricBatches[0].Sent(), jc.IsFalse)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
}

// TestCountMetrics asserts the correct values are returned
// by CountofUnsentMetrics and CountofSentMetrics.
func (s *MetricSuite) TestCountMetrics(c *gc.C) {
	now := time.Now()
	m := []state.Metric{{Key: "pings", Value: "123", Time: now, Credentials: []byte{}}}
	s.factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	s.factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	s.factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, Time: &now, Metrics: m})
	sent, err := s.State.CountofSentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent, gc.Equals, 1)
	unsent, err := s.State.CountofUnsentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsent, gc.Equals, 2)
	c.Assert(unsent+sent, gc.Equals, 3)
}

func (s *MetricSuite) TestSetMetricBatchesSent(c *gc.C) {
	now := time.Now()
	metrics := make([]*state.MetricBatch, 3)
	for i := range metrics {
		m := []state.Metric{{Key: "pings", Value: "123", Time: now, Credentials: []byte{}}}
		metrics[i] = s.factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	}
	uuids := make([]string, len(metrics))
	for i, m := range metrics {
		uuids[i] = m.UUID()
	}
	err := s.State.SetMetricBatchesSent(uuids)
	c.Assert(err, jc.ErrorIsNil)
	sent, err := s.State.CountofSentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent, gc.Equals, 3)

}

func (s *MetricSuite) TestMetricsToSend(c *gc.C) {
	now := state.NowToTheSecond()
	m := []state.Metric{{Key: "pings", Value: "123", Time: now, Credentials: []byte{}}}
	s.factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	s.factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	s.factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, Time: &now, Metrics: m})
	result, err := s.State.MetricsToSend(5)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 2)
}

// TestMetricsToSendBatches checks that metrics are properly batched.
func (s *MetricSuite) TestMetricsToSendBatches(c *gc.C) {
	now := state.NowToTheSecond()
	for i := 0; i < 6; i++ {
		m := []state.Metric{{Key: "pings", Value: "123", Time: now, Credentials: []byte{}}}
		s.factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	}
	for i := 0; i < 4; i++ {
		m := []state.Metric{{Key: "pings", Value: "123", Time: now, Credentials: []byte{}}}
		s.factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, Time: &now, Metrics: m})
	}
	for i := 0; i < 3; i++ {
		result, err := s.State.MetricsToSend(2)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, gc.HasLen, 2)
		uuids := make([]string, len(result))
		for i, m := range result {
			uuids[i] = m.UUID()
		}
		s.State.SetMetricBatchesSent(uuids)
	}
	result, err := s.State.MetricsToSend(2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)
}

func (s *MetricSuite) TestMetricValidation(c *gc.C) {
	nonMeteredUnit := s.factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	meteredService := s.factory.MakeService(c, &factory.ServiceParams{Name: "metered-service", Charm: s.meteredCharm})
	meteredUnit := s.factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	dyingUnit, err := meteredService.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = dyingUnit.SetCharmURL(s.meteredCharm.URL())
	c.Assert(err, jc.ErrorIsNil)
	err = dyingUnit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	tests := []struct {
		about   string
		metrics []state.Metric
		unit    *state.Unit
		err     string
	}{{
		"assert non metered unit returns an error",
		[]state.Metric{{"metric-key", "1", now, []byte("creds")}},
		nonMeteredUnit,
		"charm doesn't implement metrics",
	}, {
		"assert metric with no errors and passes validation",
		[]state.Metric{{"pings", "1", now, []byte("creds")}},
		meteredUnit,
		"",
	}, {
		"assert valid metric fails on dying unit",
		[]state.Metric{{"pings", "1", now, []byte("creds")}},
		dyingUnit,
		"metered-service/1 not found",
	}, {
		"assert charm doesn't implement key returns error",
		[]state.Metric{{"not-implemented", "1", now, []byte("creds")}},
		meteredUnit,
		`metric "not-implemented" not defined`,
	}, {
		"assert invalid value returns error",
		[]state.Metric{{"pings", "foobar", now, []byte("creds")}},
		meteredUnit,
		`invalid value type: expected float, got "foobar"`,
	}, {
		"long value returns error",
		[]state.Metric{{"pings", "3.141592653589793238462643383279", now, []byte("creds")}},
		meteredUnit,
		`metric value is too large`,
	}}
	for i, t := range tests {
		c.Logf("test %d: %s", i, t.about)
		_, err := t.unit.AddMetrics(now, t.metrics)
		if t.err == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
	}
}
