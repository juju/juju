// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type MetricSuite struct {
	ConnSuite
	unit         *state.Unit
	service      *state.Service
	meteredCharm *state.Charm
}

var _ = gc.Suite(&MetricSuite{})

func (s *MetricSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.assertAddUnit(c)
}

func (s *MetricSuite) TestAddNoMetrics(c *gc.C) {
	now := state.NowToTheSecond()
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

func (s *MetricSuite) assertAddUnit(c *gc.C) {
	s.meteredCharm = s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	s.service = s.Factory.MakeService(c, &factory.ServiceParams{Charm: s.meteredCharm})
	s.unit = s.Factory.MakeUnit(c, &factory.UnitParams{Service: s.service, SetCharmURL: true})
}

func (s *MetricSuite) TestAddMetric(c *gc.C) {
	now := state.NowToTheSecond()
	modelUUID := s.State.ModelUUID()
	m := state.Metric{"pings", "5", now}
	metricBatch, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatch.Unit(), gc.Equals, "metered/0")
	c.Assert(metricBatch.ModelUUID(), gc.Equals, modelUUID)
	c.Assert(metricBatch.CharmURL(), gc.Equals, "cs:quantal/metered")
	c.Assert(metricBatch.Sent(), jc.IsFalse)
	c.Assert(metricBatch.Created(), gc.Equals, now)
	c.Assert(metricBatch.Metrics(), gc.HasLen, 1)

	metric := metricBatch.Metrics()[0]
	c.Assert(metric.Key, gc.Equals, "pings")
	c.Assert(metric.Value, gc.Equals, "5")
	c.Assert(metric.Time.Equal(now), jc.IsTrue)

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
}

func (s *MetricSuite) TestAddMetricNonExistentUnit(c *gc.C) {
	removeUnit(c, s.unit)
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now}
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
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now}
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
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now}
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
	err = saved.SetSent(time.Now())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Sent(), jc.IsTrue)
	saved, err = s.State.MetricBatch(added.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Sent(), jc.IsTrue)
}

func (s *MetricSuite) TestCleanupMetrics(c *gc.C) {
	oldTime := time.Now().Add(-(time.Hour * 25))
	now := time.Now()
	m := state.Metric{"pings", "5", oldTime}
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
	oldMetric1.SetSent(time.Now().Add(-25 * time.Hour))

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
	oldMetric2.SetSent(time.Now().Add(-25 * time.Hour))

	m = state.Metric{"pings", "5", now}
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
	newMetric.SetSent(time.Now())
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
	oldTime := time.Now().Add(-(time.Hour * 25))
	m := state.Metric{"pings", "5", oldTime}
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

	now := time.Now()
	m = state.Metric{"pings", "5", now}
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
	newMetric.SetSent(time.Now())
	err = s.State.CleanupOldMetrics()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.MetricBatch(newMetric.UUID())
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.MetricBatch(oldMetric.UUID())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MetricSuite) TestAllMetricBatches(c *gc.C) {
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now}
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
	c.Assert(metricBatches[0].CharmURL(), gc.Equals, "cs:quantal/metered")
	c.Assert(metricBatches[0].Sent(), jc.IsFalse)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
}

func (s *MetricSuite) TestAllMetricBatchesCustomCharmURLAndUUID(c *gc.C) {
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now}
	uuid := utils.MustNewUUID().String()
	charmUrl := "cs:quantal/metered"
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     uuid,
			Created:  now,
			CharmURL: charmUrl,
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
	c.Assert(metricBatches[0].CharmURL(), gc.Equals, charmUrl)
	c.Assert(metricBatches[0].Sent(), jc.IsFalse)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
}

func (s *MetricSuite) TestMetricCredentials(c *gc.C) {
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now}
	err := s.service.SetMetricCredentials([]byte("hello there"))
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
	now := time.Now()
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
	now := time.Now()
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
	now := state.NowToTheSecond()
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
	now := state.NowToTheSecond()
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
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Name: "metered-service", Charm: s.meteredCharm})
	meteredUnit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
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
		[]state.Metric{{"metric-key", "1", now}},
		nonMeteredUnit,
		"charm doesn't implement metrics",
	}, {
		"assert metric with no errors and passes validation",
		[]state.Metric{{"pings", "1", now}},
		meteredUnit,
		"",
	}, {
		"assert valid metric fails on dying unit",
		[]state.Metric{{"pings", "1", now}},
		dyingUnit,
		"unit \"metered-service/1\" not found",
	}, {
		"assert charm doesn't implement key returns error",
		[]state.Metric{{"not-implemented", "1", now}},
		meteredUnit,
		`metric "not-implemented" not defined`,
	}, {
		"assert invalid value returns error",
		[]state.Metric{{"pings", "foobar", now}},
		meteredUnit,
		`invalid value type: expected float, got "foobar"`,
	}, {
		"long value returns error",
		[]state.Metric{{"pings", "3.141592653589793238462643383279", now}},
		meteredUnit,
		`metric value is too large`,
	}, {
		"negative value returns error",
		[]state.Metric{{"pings", "-42.0", now}},
		meteredUnit,
		`invalid value: value must be greater or equal to zero, got -42.0`,
	}, {
		"non-float value returns an error",
		[]state.Metric{{"pings", "abcd", now}},
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

func (s *MetricSuite) TestMetricsAcrossEnvironments(c *gc.C) {
	now := state.NowToTheSecond().Add(-48 * time.Hour)
	m := state.Metric{"pings", "5", now}
	m1, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st)
	meteredCharm := f.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	service := f.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := f.MakeUnit(c, &factory.UnitParams{Service: service, SetCharmURL: true})
	m2, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     utils.MustNewUUID().String(),
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{m},
			Unit:     unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	batches, err := s.State.AllMetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 2)

	unsent, err := s.State.CountOfUnsentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsent, gc.Equals, 2)

	toSend, err := s.State.MetricsToSend(10)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(toSend, gc.HasLen, 2)

	err = m1.SetSent(time.Now().Add(-25 * time.Hour))
	c.Assert(err, jc.ErrorIsNil)
	err = m2.SetSent(time.Now().Add(-25 * time.Hour))
	c.Assert(err, jc.ErrorIsNil)

	sent, err := s.State.CountOfSentMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sent, gc.Equals, 2)

	err = s.State.CleanupOldMetrics()
	c.Assert(err, jc.ErrorIsNil)

	batches, err = s.State.AllMetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 0)
}

func (s *MetricSuite) TestAddMetricDuplicateUUID(c *gc.C) {
	now := state.NowToTheSecond()
	mUUID := utils.MustNewUUID().String()
	_, err := s.State.AddMetrics(
		state.BatchParam{
			UUID:     mUUID,
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{{"pings", "5", now}},
			Unit:     s.unit.UnitTag(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMetrics(
		state.BatchParam{
			UUID:     mUUID,
			Created:  now,
			CharmURL: s.meteredCharm.URL().String(),
			Metrics:  []state.Metric{{"pings", "10", now}},
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
		now := state.NowToTheSecond()
		modelUUID := s.State.ModelUUID()
		m := state.Metric{"juju-units", test.value, now}
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
			c.Assert(metricBatch.CharmURL(), gc.Equals, "cs:quantal/metered")
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
			c.Assert(metricBatch.CharmURL(), gc.Equals, "cs:quantal/metered")
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

func (s *MetricSuite) TestUnitMetricBatchesReturnsJustLocal(c *gc.C) {
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now}
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
	localMeteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	service := s.Factory.MakeService(c, &factory.ServiceParams{Name: "localmetered", Charm: localMeteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: service, SetCharmURL: true})
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
	c.Assert(metricBatches, gc.HasLen, 0)
	metricBatches, err = s.State.MetricBatchesForUnit("localmetered/0")
	c.Assert(metricBatches, gc.HasLen, 1)
}

type MetricLocalCharmSuite struct {
	ConnSuite
	unit         *state.Unit
	service      *state.Service
	meteredCharm *state.Charm
}

var _ = gc.Suite(&MetricLocalCharmSuite{})

func (s *MetricLocalCharmSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.assertAddLocalUnit(c)
}

func (s *MetricLocalCharmSuite) assertAddLocalUnit(c *gc.C) {
	s.meteredCharm = s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	s.service = s.Factory.MakeService(c, &factory.ServiceParams{Charm: s.meteredCharm})
	s.unit = s.Factory.MakeUnit(c, &factory.UnitParams{Service: s.service, SetCharmURL: true})
}

func (s *MetricLocalCharmSuite) TestUnitMetricBatches(c *gc.C) {
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now}
	m2 := state.Metric{"pings", "10", now}
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
	newUnit, err := s.service.AddUnit()
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
	c.Assert(metricBatches[0].Unit(), gc.Equals, "metered/0")
	c.Assert(metricBatches[0].CharmURL(), gc.Equals, "local:quantal/metered")
	c.Assert(metricBatches[0].Sent(), jc.IsFalse)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
	c.Assert(metricBatches[0].Metrics()[0].Value, gc.Equals, "5")

	metricBatches, err = s.State.MetricBatchesForUnit("metered/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	c.Assert(metricBatches[0].Unit(), gc.Equals, "metered/1")
	c.Assert(metricBatches[0].CharmURL(), gc.Equals, "local:quantal/metered")
	c.Assert(metricBatches[0].Sent(), jc.IsFalse)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
	c.Assert(metricBatches[0].Metrics()[0].Value, gc.Equals, "10")
}

func (s *MetricLocalCharmSuite) TestServiceMetricBatches(c *gc.C) {
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now}
	m2 := state.Metric{"pings", "10", now}
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
	newUnit, err := s.service.AddUnit()
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

	metricBatches, err := s.State.MetricBatchesForService("metered")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 2)

	c.Assert(metricBatches[0].Unit(), gc.Equals, "metered/0")
	c.Assert(metricBatches[0].CharmURL(), gc.Equals, "local:quantal/metered")
	c.Assert(metricBatches[0].Sent(), jc.IsFalse)
	c.Assert(metricBatches[0].Metrics(), gc.HasLen, 1)
	c.Assert(metricBatches[0].Metrics()[0].Value, gc.Equals, "5")

	c.Assert(metricBatches[1].Unit(), gc.Equals, "metered/1")
	c.Assert(metricBatches[1].CharmURL(), gc.Equals, "local:quantal/metered")
	c.Assert(metricBatches[1].Sent(), jc.IsFalse)
	c.Assert(metricBatches[1].Metrics(), gc.HasLen, 1)
	c.Assert(metricBatches[1].Metrics()[0].Value, gc.Equals, "10")
}

func (s *MetricLocalCharmSuite) TestUnitMetricBatchesReturnsJustLocal(c *gc.C) {
	now := state.NowToTheSecond()
	m := state.Metric{"pings", "5", now}
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
	csMeteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	service := s.Factory.MakeService(c, &factory.ServiceParams{Name: "csmetered", Charm: csMeteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: service, SetCharmURL: true})
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
	c.Assert(metricBatches, gc.HasLen, 1)
	metricBatches, err = s.State.MetricBatchesForUnit("csmetered/0")
	c.Assert(metricBatches, gc.HasLen, 0)
}
