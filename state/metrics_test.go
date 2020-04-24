// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type MetricSuite struct {
	ConnSuite
	unit         *state.Unit
	application  *state.Application
	meteredCharm *state.Charm
}

var _ = gc.Suite(&MetricSuite{})

func (s *MetricSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.meteredCharm = s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered-1"})
	s.application = s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: s.meteredCharm})
	s.unit = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application, SetCharmURL: true})
}

func (s *MetricSuite) TestAddNoMetrics(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	_, err := s.State.AddMetrics(state.BatchParam{
		UUID:     utils.MustNewUUID().String(),
		CharmURL: s.meteredCharm.URL().String(),
		Created:  now,
		Metrics:  []state.Metric{},
		Unit:     s.unit.UnitTag(),
	})
	c.Assert(err, gc.ErrorMatches, "cannot add a batch of 0 metrics")
}

func removeUnit(c *gc.C, unit *state.Unit) {
	ensureUnitDead(c, unit)
	err := unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func ensureUnitDead(c *gc.C, unit *state.Unit) {
	err := unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricSuite) TestAddMetric(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	modelUUID := s.State.ModelUUID()
	m := []state.Metric{{
		Key: "pings", Value: "5", Time: now,
	}, {
		Key: "pongs", Value: "6", Time: now, Labels: map[string]string{"foo": "bar"},
	}}
	metricBatch, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  m,
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatch.Unit(), gc.Equals, "metered/0")
	c.Assert(metricBatch.ModelUUID(), gc.Equals, modelUUID)
	c.Assert(metricBatch.CharmURL(), gc.Equals, "cs:quantal/metered-1")
	c.Assert(metricBatch.Sent(), jc.IsFalse)
	c.Assert(metricBatch.Created(), gc.Equals, now)
	c.Assert(metricBatch.Metrics(), gc.HasLen, 2)

	metric := metricBatch.Metrics()[0]
	c.Assert(metric.Key, gc.Equals, "pings")
	c.Assert(metric.Value, gc.Equals, "5")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	metric = metricBatch.Metrics()[1]
	c.Assert(metric.Key, gc.Equals, "pongs")
	c.Assert(metric.Value, gc.Equals, "6")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	c.Assert(metric.Labels, gc.DeepEquals, map[string]string{"foo": "bar"})

	saved, err := s.State.MetricBatch(metricBatch.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Unit(), gc.Equals, "metered/0")
	c.Assert(metricBatch.CharmURL(), gc.Equals, "cs:quantal/metered-1")
	c.Assert(saved.Sent(), jc.IsFalse)
	c.Assert(saved.Metrics(), gc.HasLen, 2)
	metric = saved.Metrics()[0]
	c.Assert(metric.Key, gc.Equals, "pings")
	c.Assert(metric.Value, gc.Equals, "5")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	metric = saved.Metrics()[1]
	c.Assert(metric.Key, gc.Equals, "pongs")
	c.Assert(metric.Value, gc.Equals, "6")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	c.Assert(metric.Labels, gc.DeepEquals, map[string]string{"foo": "bar"})
}

func (s *MetricSuite) TestAddMetricOrderedLabels(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	modelUUID := s.State.ModelUUID()
	m := []state.Metric{{
		Key: "pings", Value: "6", Time: now,
	}, {
		Key: "pings", Value: "1", Time: now, Labels: map[string]string{"quux": "baz"},
	}, {
		Key: "pings", Value: "2", Time: now, Labels: map[string]string{"abc": "123"},
	}, {
		Key: "pings", Value: "3", Time: now, Labels: map[string]string{"foo": "bar"},
	}}
	metricBatch, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  m,
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatch.Unit(), gc.Equals, "metered/0")
	c.Assert(metricBatch.ModelUUID(), gc.Equals, modelUUID)
	c.Assert(metricBatch.CharmURL(), gc.Equals, "cs:quantal/metered-1")
	c.Assert(metricBatch.Sent(), jc.IsFalse)
	c.Assert(metricBatch.Created(), gc.Equals, now)
	uniqueMetrics := metricBatch.UniqueMetrics()
	c.Assert(uniqueMetrics, gc.HasLen, 4)
	c.Assert(uniqueMetrics, gc.DeepEquals, []state.Metric{{
		Key: "pings", Value: "6", Time: now,
	}, {
		Key: "pings", Value: "2", Time: now, Labels: map[string]string{"abc": "123"},
	}, {
		Key: "pings", Value: "3", Time: now, Labels: map[string]string{"foo": "bar"},
	}, {
		Key: "pings", Value: "1", Time: now, Labels: map[string]string{"quux": "baz"},
	}})
}

func (s *MetricSuite) TestAddModelMetricMetric(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	modelUUID := s.State.ModelUUID()
	m := []state.Metric{{
		Key: "pings", Value: "5", Time: now,
	}, {
		Key: "pongs", Value: "6", Time: now, Labels: map[string]string{"foo": "bar"},
	}}
	metricBatch, err := s.State.AddModelMetrics(
		state.ModelBatchParam{
			UUID:    utils.MustNewUUID().String(),
			Created: now,
			Metrics: m,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatch.Unit(), gc.Equals, "")
	c.Assert(metricBatch.ModelUUID(), gc.Equals, modelUUID)
	c.Assert(metricBatch.CharmURL(), gc.Equals, "")
	c.Assert(metricBatch.Sent(), jc.IsFalse)
	c.Assert(metricBatch.Created(), gc.Equals, now)
	c.Assert(metricBatch.Metrics(), gc.HasLen, 2)

	metric := metricBatch.Metrics()[0]
	c.Assert(metric.Key, gc.Equals, "pings")
	c.Assert(metric.Value, gc.Equals, "5")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	metric = metricBatch.Metrics()[1]
	c.Assert(metric.Key, gc.Equals, "pongs")
	c.Assert(metric.Value, gc.Equals, "6")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	c.Assert(metric.Labels, gc.DeepEquals, map[string]string{"foo": "bar"})

	tosend, err := s.State.MetricsToSend(1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tosend, gc.HasLen, 1)
	saved := tosend[0]
	c.Assert(saved.Unit(), gc.Equals, "")
	c.Assert(metricBatch.CharmURL(), gc.Equals, "")
	c.Assert(saved.Sent(), jc.IsFalse)
	c.Assert(saved.Metrics(), gc.HasLen, 2)
	metric = saved.Metrics()[0]
	c.Assert(metric.Key, gc.Equals, "pings")
	c.Assert(metric.Value, gc.Equals, "5")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	metric = saved.Metrics()[1]
	c.Assert(metric.Key, gc.Equals, "pongs")
	c.Assert(metric.Value, gc.Equals, "6")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)
	c.Assert(metric.Labels, gc.DeepEquals, map[string]string{"foo": "bar"})
}

func (s *MetricSuite) TestAddMetricNonExistentUnit(c *gc.C) {
	removeUnit(c, s.unit)
	now := state.NowToTheSecond(s.State)
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	unitTag := names.NewUnitTag("test/0")
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     unitTag,
		},
	)
	c.Assert(err, gc.ErrorMatches, ".*not found")
}

func (s *MetricSuite) TestAddMetricDeadUnit(c *gc.C) {
	ensureUnitDead(c, s.unit)
	now := state.NowToTheSecond(s.State)
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, gc.ErrorMatches, `metered/0 not found`)
}

func (s *MetricSuite) TestSetMetricSent(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	added, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	saved, err := s.State.MetricBatch(added.UUID())
	c.Assert(err, jc.ErrorIsNil)
	err = saved.SetSent(testing.NonZeroTime())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Sent(), jc.IsTrue)
	saved, err = s.State.MetricBatch(added.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Sent(), jc.IsTrue)
}

func (s *MetricSuite) TestCleanupMetrics(c *gc.C) {
	oldTime := testing.NonZeroTime().Add(-(time.Hour * 25))
	now := testing.NonZeroTime()
	m := state.Metric{Key: "pings", Value: "5", Time: oldTime}
	oldMetric1, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	oldMetric1.SetSent(testing.NonZeroTime().Add(-25 * time.Hour))

	oldMetric2, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	oldMetric2.SetSent(testing.NonZeroTime().Add(-25 * time.Hour))

	m = state.Metric{Key: "pings", Value: "5", Time: now}
	newMetric, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	newMetric.SetSent(testing.NonZeroTime())
	err = s.State.CleanupOldMetrics()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.MetricBatch(newMetric.UUID())
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.MetricBatch(oldMetric1.UUID())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.State.MetricBatch(oldMetric2.UUID())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *MetricSuite) TestCleanupNoMetrics(c *gc.C) {
	err := s.State.CleanupOldMetrics()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricSuite) TestCleanupMetricsIgnoreNotSent(c *gc.C) {
	oldTime := testing.NonZeroTime().Add(-(time.Hour * 25))
	m := state.Metric{Key: "pings", Value: "5", Time: oldTime}
	oldMetric, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  oldTime,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	now := testing.NonZeroTime()
	m = state.Metric{Key: "pings", Value: "5", Time: now}
	newMetric, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	newMetric.SetSent(testing.NonZeroTime())
	err = s.State.CleanupOldMetrics()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.MetricBatch(newMetric.UUID())
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.MetricBatch(oldMetric.UUID())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricSuite) TestAllMetricBatches(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	metricBatches, err := s.State.AllMetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	c.Assert(metricBatches[0].Unit(), gc.Equals, "metered/0")
	c.Assert(metricBatches[0].CharmURL(), gc.Equals, "cs:quantal/metered-1")
	c.Assert(metricBatches[0].Sent(), jc.IsFalse)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
}

func (s *MetricSuite) TestAllMetricBatchesCustomCharmURLAndUUID(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	uuid := utils.MustNewUUID().String()
	charmURL := "cs:quantal/metered-1"
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     uuid,
			Created:  now,
			CharmURL: charmURL,
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	metricBatches, err := s.State.AllMetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	c.Assert(metricBatches[0].Unit(), gc.Equals, "metered/0")
	c.Assert(metricBatches[0].UUID(), gc.Equals, uuid)
	c.Assert(metricBatches[0].CharmURL(), gc.Equals, charmURL)
	c.Assert(metricBatches[0].Sent(), jc.IsFalse)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
}

func (s *MetricSuite) TestMetricCredentials(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	err := s.application.SetMetricCredentials([]byte("hello there"))
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	metricBatches, err := s.State.AllMetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	c.Assert(metricBatches[0].Credentials(), gc.DeepEquals, []byte("hello there"))
}

// TestCountMetrics asserts the correct values are returned
// by CountOfUnsentMetrics and CountOfSentMetrics.
func (s *MetricSuite) TestCountMetrics(c *gc.C) {
	now := testing.NonZeroTime()
	m := []state.Metric{{Key: "pings", Value: "123", Time: now}}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, Time: &now, Metrics: m})
	sent, err := s.State.CountOfSentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent, gc.Equals, 1)
	unsent, err := s.State.CountOfUnsentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsent, gc.Equals, 2)
	c.Assert(unsent+sent, gc.Equals, 3)
}

func (s *MetricSuite) TestSetMetricBatchesSent(c *gc.C) {
	now := testing.NonZeroTime()
	metrics := make([]*state.MetricBatch, 3)
	for i := range metrics {
		m := []state.Metric{{Key: "pings", Value: "123", Time: now}}
		metrics[i] = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	}
	uuids := make([]string, len(metrics))
	for i, m := range metrics {
		uuids[i] = m.UUID()
	}
	err := s.State.SetMetricBatchesSent(uuids)
	c.Assert(err, jc.ErrorIsNil)
	sent, err := s.State.CountOfSentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent, gc.Equals, 3)

}

func (s *MetricSuite) TestMetricsToSend(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	m := []state.Metric{{Key: "pings", Value: "123", Time: now}}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, Time: &now, Metrics: m})
	result, err := s.State.MetricsToSend(5)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 2)
}

// TestMetricsToSendBatches checks that metrics are properly batched.
func (s *MetricSuite) TestMetricsToSendBatches(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	for i := 0; i < 6; i++ {
		m := []state.Metric{{Key: "pings", Value: "123", Time: now}}
		s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: m})
	}
	for i := 0; i < 4; i++ {
		m := []state.Metric{{Key: "pings", Value: "123", Time: now}}
		s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, Time: &now, Metrics: m})
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
	nonMeteredUnit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	meteredApplication := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "metered-application", Charm: s.meteredCharm})
	meteredUnit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredApplication, SetCharmURL: true})
	dyingUnit, err := meteredApplication.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = dyingUnit.SetCharmURL(s.meteredCharm.URL())
	c.Assert(err, jc.ErrorIsNil)
	err = dyingUnit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	now := testing.NonZeroTime()
	tests := []struct {
		about   string
		metrics []state.Metric
		unit    *state.Unit
		err     string
	}{{
		"assert non metered unit returns an error",
		[]state.Metric{{Key: "metric-key", Value: "1", Time: now}},
		nonMeteredUnit,
		"charm doesn't implement metrics",
	}, {
		"assert metric with no errors and passes validation",
		[]state.Metric{{Key: "pings", Value: "1", Time: now}},
		meteredUnit,
		"",
	}, {
		"assert valid metric fails on dying unit",
		[]state.Metric{{Key: "pings", Value: "1", Time: now}},
		dyingUnit,
		"unit \"metered-application/1\" not found",
	}, {
		"assert charm doesn't implement key returns error",
		[]state.Metric{{Key: "not-implemented", Value: "1", Time: now}},
		meteredUnit,
		`metric "not-implemented" not defined`,
	}, {
		"assert invalid value returns error",
		[]state.Metric{{Key: "pings", Value: "foobar", Time: now}},
		meteredUnit,
		`invalid value type: expected float, got "foobar"`,
	}, {
		"long value returns error",
		[]state.Metric{{Key: "pings", Value: "3.141592653589793238462643383279", Time: now}},
		meteredUnit,
		`metric value is too large`,
	}, {
		"negative value returns error",
		[]state.Metric{{Key: "pings", Value: "-42.0", Time: now}},
		meteredUnit,
		`invalid value: value must be greater or equal to zero, got -42.0`,
	}, {
		"non-float value returns an error",
		[]state.Metric{{Key: "pings", Value: "abcd", Time: now}},
		meteredUnit,
		`invalid value type: expected float, got "abcd"`,
	}}
	for i, t := range tests {
		c.Logf("test %d: %s", i, t.about)
		chURL, ok := t.unit.CharmURL()
		c.Assert(ok, jc.IsTrue)
		_, err := s.State.AddMetrics(
			state.BatchParam{
				UUID:     utils.MustNewUUID().String(),
				Created:  now,
				CharmURL: chURL.String(),
				Metrics:  t.metrics,
				Unit:     t.unit.UnitTag(),
			},
		)
		if t.err == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
	}
}

func (s *MetricSuite) TestAddMetricDuplicateUUID(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	mUUID := utils.MustNewUUID().String()
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     mUUID,
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{{Key: "pings", Value: "5", Time: now}},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMetrics(
		state.BatchParam{
			UUID:     mUUID,
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{{Key: "pings", Value: "10", Time: now}},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, gc.ErrorMatches, "metrics batch .* already exists")
}

func (s *MetricSuite) TestAddBuiltInMetric(c *gc.C) {
	tests := []struct {
		about         string
		value         string
		expectedError string
	}{{
		about: "adding a positive value must succeed",
		value: "5",
	}, {
		about:         "negative values return an error",
		value:         "-42.0",
		expectedError: "invalid value: value must be greater or equal to zero, got -42.0",
	}, {
		about:         "non-float values return an error",
		value:         "abcd",
		expectedError: `invalid value type: expected float, got "abcd"`,
	}, {
		about:         "long values return an error",
		value:         "1234567890123456789012345678901234567890",
		expectedError: "metric value is too large",
	},
	}
	for _, test := range tests {
		c.Logf("running test: %v", test.about)
		now := state.NowToTheSecond(s.State)
		modelUUID := s.State.ModelUUID()
		m := state.Metric{Key: "juju-units", Value: test.value, Time: now}
		metricBatch, err := s.State.AddMetrics(
			state.BatchParam{
				UUID:     utils.MustNewUUID().String(),
				Created:  now,
				CharmURL: s.meteredCharm.URL().String(),
				Metrics:  []state.Metric{m},
				Unit:     s.unit.UnitTag(),
			},
		)
		if test.expectedError == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(metricBatch.Unit(), gc.Equals, "metered/0")
			c.Assert(metricBatch.ModelUUID(), gc.Equals, modelUUID)
			c.Assert(metricBatch.CharmURL(), gc.Equals, "cs:quantal/metered-1")
			c.Assert(metricBatch.Sent(), jc.IsFalse)
			c.Assert(metricBatch.Created(), gc.Equals, now)
			c.Assert(metricBatch.Metrics(), gc.HasLen, 1)

			metric := metricBatch.Metrics()[0]
			c.Assert(metric.Key, gc.Equals, "juju-units")
			c.Assert(metric.Value, gc.Equals, test.value)
			c.Assert(metric.Time.Equal(now), jc.IsTrue)

			saved, err := s.State.MetricBatch(metricBatch.UUID())
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(saved.Unit(), gc.Equals, "metered/0")
			c.Assert(metricBatch.CharmURL(), gc.Equals, "cs:quantal/metered-1")
			c.Assert(saved.Sent(), jc.IsFalse)
			c.Assert(saved.Metrics(), gc.HasLen, 1)
			metric = saved.Metrics()[0]
			c.Assert(metric.Key, gc.Equals, "juju-units")
			c.Assert(metric.Value, gc.Equals, test.value)
			c.Assert(metric.Time.Equal(now), jc.IsTrue)
		} else {
			c.Assert(err, gc.ErrorMatches, test.expectedError)
		}
	}
}

func (s *MetricSuite) TestUnitMetricBatchesMatchesAllCharms(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	localMeteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "localmetered", Charm: localMeteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: application, SetCharmURL: true})
	_, err = s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: localMeteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     unit.UnitTag(),
		},
	)

	c.Assert(err, jc.ErrorIsNil)
	metricBatches, err := s.State.MetricBatchesForUnit("metered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	metricBatches, err = s.State.MetricBatchesForUnit("localmetered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
}

func (s *MetricSuite) TestNoSuchUnitMetricBatches(c *gc.C) {
	_, err := s.State.MetricBatchesForUnit("chimerical-unit/0")
	c.Assert(err, gc.ErrorMatches, `unit "chimerical-unit/0" not found`)
}

func (s *MetricSuite) TestNoSuchApplicationMetricBatches(c *gc.C) {
	_, err := s.State.MetricBatchesForApplication("unicorn-app")
	c.Assert(err, gc.ErrorMatches, `application "unicorn-app" not found`)
}

type MetricLocalCharmSuite struct {
	ConnSuite
	unit         *state.Unit
	application  *state.Application
	meteredCharm *state.Charm
}

var _ = gc.Suite(&MetricLocalCharmSuite{})

func (s *MetricLocalCharmSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.meteredCharm = s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	s.application = s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: s.meteredCharm})
	s.unit = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application, SetCharmURL: true})
}

func (s *MetricLocalCharmSuite) TestUnitMetricBatches(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	m2 := state.Metric{Key: "pings", Value: "10", Time: now}
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	newUnit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m2},
			Unit:     newUnit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	metricBatches, err := s.State.MetricBatchesForUnit("metered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	c.Check(metricBatches[0].Unit(), gc.Equals, "metered/0")
	c.Check(metricBatches[0].CharmURL(), gc.Equals, "local:quantal/metered-1")
	c.Check(metricBatches[0].Sent(), jc.IsFalse)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
	c.Check(metricBatches[0].Metrics()[0].Value, gc.Equals, "5")

	metricBatches, err = s.State.MetricBatchesForUnit("metered/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	c.Check(metricBatches[0].Unit(), gc.Equals, "metered/1")
	c.Check(metricBatches[0].CharmURL(), gc.Equals, "local:quantal/metered-1")
	c.Check(metricBatches[0].Sent(), jc.IsFalse)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
	c.Check(metricBatches[0].Metrics()[0].Value, gc.Equals, "10")
}

func (s *MetricLocalCharmSuite) TestApplicationMetricBatches(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	m2 := state.Metric{Key: "pings", Value: "10", Time: now}
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	newUnit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m2},
			Unit:     newUnit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	metricBatches, err := s.State.MetricBatchesForApplication("metered")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 2)

	c.Check(metricBatches[0].Unit(), gc.Equals, "metered/0")
	c.Check(metricBatches[0].CharmURL(), gc.Equals, "local:quantal/metered-1")
	c.Check(metricBatches[0].Sent(), jc.IsFalse)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
	c.Check(metricBatches[0].Metrics()[0].Value, gc.Equals, "5")

	c.Check(metricBatches[1].Unit(), gc.Equals, "metered/1")
	c.Check(metricBatches[1].CharmURL(), gc.Equals, "local:quantal/metered-1")
	c.Check(metricBatches[1].Sent(), jc.IsFalse)
	c.Assert(metricBatches[1].Metrics(), gc.HasLen, 1)
	c.Check(metricBatches[1].Metrics()[0].Value, gc.Equals, "10")
}

func (s *MetricLocalCharmSuite) TestModelMetricBatches(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	// Add 2 metric batches to a single unit.
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	m2 := state.Metric{Key: "pings", Value: "10", Time: now}
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	newUnit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now.Add(time.Second),
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m2},
			Unit:     newUnit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Create a new model and add a metric batch.
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	meteredCharm := f.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered-1"})
	application := f.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	unit := f.MakeUnit(c, &factory.UnitParams{Application: application, SetCharmURL: true})
	_, err = st.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// We expect 2 metric batches in our first model.
	metricBatches, err := s.State.MetricBatchesForModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 2)

	var first, second state.MetricBatch
	for _, m := range metricBatches {
		if m.Unit() == "metered/0" {
			first = m
		}
		if m.Unit() == "metered/1" {
			second = m
		}
	}
	c.Assert(first, gc.NotNil)
	c.Assert(second, gc.NotNil)

	c.Check(first.Unit(), gc.Equals, "metered/0")
	c.Check(first.CharmURL(), gc.Equals, "local:quantal/metered-1")
	c.Check(first.ModelUUID(), gc.Equals, s.State.ModelUUID())
	c.Check(first.Sent(), jc.IsFalse)
	c.Assert(first.Metrics(), gc.HasLen, 1)
	c.Check(first.Metrics()[0].Value, gc.Equals, "5")

	c.Check(second.Unit(), gc.Equals, "metered/1")
	c.Check(second.CharmURL(), gc.Equals, "local:quantal/metered-1")
	c.Check(second.ModelUUID(), gc.Equals, s.State.ModelUUID())
	c.Check(second.Sent(), jc.IsFalse)
	c.Assert(second.Metrics(), gc.HasLen, 1)
	c.Check(second.Metrics()[0].Value, gc.Equals, "10")

	// And a single metric batch in the second model.
	metricBatches, err = st.MetricBatchesForModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
}

func (s *MetricLocalCharmSuite) TestMetricsSorted(c *gc.C) {
	newUnit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	t0 := time.Date(2016, time.August, 16, 16, 00, 35, 0, time.Local)
	var times []time.Time
	for i := 0; i < 3; i++ {
		times = append(times, t0.Add(time.Minute*time.Duration(i)))
	}

	for _, t := range times {
		_, err := s.State.AddMetrics(
			state.BatchParam{
				UUID:     utils.MustNewUUID().String(),
				Created:  t,
				CharmURL: s.meteredCharm.URL().String(),
				Metrics:  []state.Metric{{Key: "pings", Value: "5", Time: t}},
				Unit:     s.unit.UnitTag(),
			},
		)
		c.Assert(err, jc.ErrorIsNil)

		_, err = s.State.AddMetrics(
			state.BatchParam{
				UUID:     utils.MustNewUUID().String(),
				Created:  t,
				CharmURL: s.meteredCharm.URL().String(),
				Metrics:  []state.Metric{{Key: "pings", Value: "10", Time: t}},
				Unit:     newUnit.UnitTag(),
			},
		)
		c.Assert(err, jc.ErrorIsNil)
	}

	metricBatches, err := s.State.MetricBatchesForUnit("metered/0")
	c.Assert(err, jc.ErrorIsNil)
	assertMetricBatchesTimeAscending(c, metricBatches)

	metricBatches, err = s.State.MetricBatchesForUnit("metered/1")
	c.Assert(err, jc.ErrorIsNil)
	assertMetricBatchesTimeAscending(c, metricBatches)

	metricBatches, err = s.State.MetricBatchesForApplication("metered")
	c.Assert(err, jc.ErrorIsNil)
	assertMetricBatchesTimeAscending(c, metricBatches)

	metricBatches, err = s.State.MetricBatchesForModel()
	c.Assert(err, jc.ErrorIsNil)
	assertMetricBatchesTimeAscending(c, metricBatches)

}

func assertMetricBatchesTimeAscending(c *gc.C, batches []state.MetricBatch) {
	var tPrev time.Time

	for i := range batches {
		if i > 0 {
			afterOrEqualPrev := func(t time.Time) bool {
				return t.After(tPrev) || t.Equal(tPrev)
			}
			desc := gc.Commentf("%+v <= %+v", batches[i-1], batches[i])
			c.Assert(batches[i].Created(), jc.Satisfies, afterOrEqualPrev, desc)
			c.Assert(batches[i].Metrics(), gc.HasLen, 1)
			c.Assert(batches[i].Metrics()[0].Time, jc.Satisfies, afterOrEqualPrev, desc)
		}
		tPrev = batches[i].Created()
	}
}

func (s *MetricLocalCharmSuite) TestUnitMetricBatchesReturnsAllCharms(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	csMeteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered-1"})
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "csmetered", Charm: csMeteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: application, SetCharmURL: true})
	_, err = s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: csMeteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     unit.UnitTag(),
		},
	)

	c.Assert(err, jc.ErrorIsNil)
	metricBatches, err := s.State.MetricBatchesForUnit("metered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	metricBatches, err = s.State.MetricBatchesForUnit("csmetered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
}

func (s *MetricLocalCharmSuite) TestUnique(c *gc.C) {
	t0 := state.NowToTheSecond(s.State)
	t1 := t0.Add(time.Second)
	batch, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  t0,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics: []state.Metric{{
				Key:   "pings",
				Value: "1",
				Time:  t0,
			}, {
				Key:   "pings",
				Value: "2",
				Time:  t1,
			}, {
				Key:   "juju-units",
				Value: "1",
				Time:  t1,
			}, {
				Key:   "juju-units",
				Value: "2",
				Time:  t0,
			}},
			Unit: s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	metrics := batch.UniqueMetrics()
	c.Assert(metrics, gc.HasLen, 2)
	c.Assert(metrics, jc.DeepEquals, []state.Metric{{
		Key:   "juju-units",
		Value: "1",
		Time:  t1,
	}, {
		Key:   "pings",
		Value: "2",
		Time:  t1,
	}})
}

type modelData struct {
	state        *state.State
	application  *state.Application
	unit         *state.Unit
	meteredCharm *state.Charm
}

type CrossModelMetricSuite struct {
	ConnSuite
	models []modelData
}

var _ = gc.Suite(&CrossModelMetricSuite{})

func (s *CrossModelMetricSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	// Set up two models.
	s.models = make([]modelData, 2)
	for i := 0; i < 2; i++ {
		s.models[i] = s.mustCreateMeteredModel(c)
	}
}

func (s *CrossModelMetricSuite) mustCreateMeteredModel(c *gc.C) modelData {
	st := s.Factory.MakeModel(c, nil)
	localFactory := factory.NewFactory(st, s.StatePool)

	meteredCharm := localFactory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered-1"})
	application := localFactory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	unit := localFactory.MakeUnit(c, &factory.UnitParams{Application: application, SetCharmURL: true})
	s.AddCleanup(func(*gc.C) { st.Close() })
	return modelData{
		state:        st,
		application:  application,
		unit:         unit,
		meteredCharm: meteredCharm,
	}
}

func (s *CrossModelMetricSuite) TestMetricsAcrossmodels(c *gc.C) {
	now := state.NowToTheSecond(s.State).Add(-48 * time.Hour)
	m := state.Metric{Key: "pings", Value: "5", Time: now}
	m1, err := s.models[0].state.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.models[0].meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.models[0].unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	m2, err := s.models[1].state.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.models[1].meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.models[1].unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	batches, err := s.State.AllMetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 2)

	unsent, err := s.models[0].state.CountOfUnsentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsent, gc.Equals, 1)

	toSend, err := s.models[0].state.MetricsToSend(10)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(toSend, gc.HasLen, 1)

	err = m1.SetSent(testing.NonZeroTime().Add(-25 * time.Hour))
	c.Assert(err, jc.ErrorIsNil)
	err = m2.SetSent(testing.NonZeroTime().Add(-25 * time.Hour))
	c.Assert(err, jc.ErrorIsNil)

	sent, err := s.models[0].state.CountOfSentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent, gc.Equals, 1)

	err = s.models[0].state.CleanupOldMetrics()
	c.Assert(err, jc.ErrorIsNil)

	// The metric from model s.models[1] should still be in place.
	batches, err = s.State.AllMetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 1)
}
