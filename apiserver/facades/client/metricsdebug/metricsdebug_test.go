// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/metricsdebug"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type metricsDebugSuite struct {
	jujutesting.JujuConnSuite

	metricsdebug *metricsdebug.MetricsDebugAPI
	authorizer   apiservertesting.FakeAuthorizer
	unit         *state.Unit
}

var _ = gc.Suite(&metricsDebugSuite{})

func (s *metricsDebugSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	debug, err := metricsdebug.NewMetricsDebugAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.metricsdebug = debug
}

func (s *metricsDebugSuite) TestSetMeterStatus(c *gc.C) {
	testCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	testService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: testCharm})
	testUnit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: testService, SetCharmURL: true})
	testUnit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: testService, SetCharmURL: true})

	csCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered-1"})
	csService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "cs-service", Charm: csCharm})
	csUnit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: csService, SetCharmURL: true})

	tests := []struct {
		about  string
		params params.MeterStatusParams
		err    string
		assert func(*gc.C, params.ErrorResults)
	}{{
		about: "set application meter status",
		params: params.MeterStatusParams{
			Statuses: []params.MeterStatusParam{{
				Tag:  testService.Tag().String(),
				Code: "RED",
				Info: "test",
			},
			},
		},
		assert: func(c *gc.C, results params.ErrorResults) {
			err := results.OneError()
			c.Assert(err, jc.ErrorIsNil)
			ms1, err := testUnit1.GetMeterStatus()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(ms1, gc.DeepEquals, state.MeterStatus{
				Code: state.MeterRed,
				Info: "test",
			})
			ms2, err := testUnit2.GetMeterStatus()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(ms2, gc.DeepEquals, state.MeterStatus{
				Code: state.MeterRed,
				Info: "test",
			})
		},
	}, {
		about: "set unit meter status",
		params: params.MeterStatusParams{
			Statuses: []params.MeterStatusParam{{
				Tag:  testUnit1.Tag().String(),
				Code: "AMBER",
				Info: "test",
			},
			},
		},
		assert: func(c *gc.C, results params.ErrorResults) {
			err := results.OneError()
			c.Assert(err, jc.ErrorIsNil)
			ms1, err := testUnit1.GetMeterStatus()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(ms1, gc.DeepEquals, state.MeterStatus{
				Code: state.MeterAmber,
				Info: "test",
			})
		},
	}, {
		about: "not a local charm - application",
		params: params.MeterStatusParams{
			Statuses: []params.MeterStatusParam{{
				Tag:  csService.Tag().String(),
				Code: "AMBER",
				Info: "test",
			},
			},
		},
		assert: func(c *gc.C, results params.ErrorResults) {
			err := results.OneError()
			c.Assert(err, gc.DeepEquals, &params.Error{Message: "not a local charm"})
		},
	}, {
		about: "not a local charm - unit",
		params: params.MeterStatusParams{
			Statuses: []params.MeterStatusParam{{
				Tag:  csUnit1.Tag().String(),
				Code: "AMBER",
				Info: "test",
			},
			},
		},
		assert: func(c *gc.C, results params.ErrorResults) {
			err := results.OneError()
			c.Assert(err, gc.DeepEquals, &params.Error{Message: "not a local charm"})
		},
	}, {
		about: "invalid meter status",
		params: params.MeterStatusParams{
			Statuses: []params.MeterStatusParam{{
				Tag:  testUnit1.Tag().String(),
				Code: "WRONG",
				Info: "test",
			},
			},
		},
		assert: func(c *gc.C, results params.ErrorResults) {
			err := results.OneError()
			c.Assert(err, gc.DeepEquals, &params.Error{Message: "meter status \"NOT AVAILABLE\" not valid"})
		},
	}, {
		about: "not such application",
		params: params.MeterStatusParams{
			Statuses: []params.MeterStatusParam{{
				Tag:  "application-missing",
				Code: "AMBER",
				Info: "test",
			},
			},
		},
		assert: func(c *gc.C, results params.ErrorResults) {
			err := results.OneError()
			c.Assert(err, gc.DeepEquals, &params.Error{Message: "application \"missing\" not found", Code: "not found"})
		},
	},
	}

	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		result, err := s.metricsdebug.SetMeterStatus(test.params)
		if test.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			test.assert(c, result)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *metricsDebugSuite) TestGetMetrics(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	t0 := time.Now().Round(time.Second)
	t1 := t0.Add(time.Second)
	metricA := state.Metric{Key: "pings", Value: "5", Time: t0}
	metricB := state.Metric{Key: "pings", Value: "10.5", Time: t1}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA, metricB}})
	args := params.Entities{Entities: []params.Entity{
		{"unit-metered/0"},
	}}
	result, err := s.metricsdebug.GetMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Metrics, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.EntityMetrics{
		Metrics: []params.MetricResult{
			{
				Key:   "pings",
				Value: "10.5",
				Time:  t1,
				Unit:  "metered/0",
			},
		},
		Error: nil,
	})
}

func (s *metricsDebugSuite) TestGetMetricsLabelOrdering(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	t0 := time.Now().Round(time.Second)
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{{
		Key: "pings", Value: "6", Time: t0,
	}, {
		Key: "pings", Value: "2", Time: t0, Labels: map[string]string{"quux": "baz"},
	}, {
		Key: "pings", Value: "3", Time: t0, Labels: map[string]string{"foo": "bar"},
	}, {
		Key: "pings", Value: "1", Time: t0, Labels: map[string]string{"abc": "123"},
	}}})
	args := params.Entities{Entities: []params.Entity{
		{"unit-metered/0"},
	}}
	result, err := s.metricsdebug.GetMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Metrics, gc.HasLen, 4)
	c.Assert(result.Results[0], jc.DeepEquals, params.EntityMetrics{
		Metrics: []params.MetricResult{{
			Unit: "metered/0", Key: "pings", Value: "6", Time: t0,
		}, {
			Unit: "metered/0", Key: "pings", Value: "1", Time: t0, Labels: map[string]string{"abc": "123"},
		}, {
			Unit: "metered/0", Key: "pings", Value: "3", Time: t0, Labels: map[string]string{"foo": "bar"},
		}, {
			Unit: "metered/0", Key: "pings", Value: "2", Time: t0, Labels: map[string]string{"quux": "baz"},
		}},
		Error: nil,
	})
}

func (s *metricsDebugSuite) TestGetMetricsFiltersCorrectly(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	unit0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	t0 := time.Now().Round(time.Second)
	t1 := t0.Add(time.Second)
	metricA := state.Metric{Key: "pings", Value: "5", Time: t1}
	metricB := state.Metric{Key: "pings", Value: "10.5", Time: t0}
	metricC := state.Metric{Key: "juju-units", Value: "8", Time: t1}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit0, Metrics: []state.Metric{metricA, metricB, metricC}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit1, Metrics: []state.Metric{metricA, metricB, metricC}})
	args := params.Entities{}
	result, err := s.metricsdebug.GetMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Metrics, gc.HasLen, 4)
	c.Assert(result.Results[0].Metrics, gc.DeepEquals, []params.MetricResult{{
		Key:   "juju-units",
		Value: "8",
		Time:  t1,
		Unit:  "metered/0",
	}, {
		Key:   "pings",
		Value: "5",
		Time:  t1,
		Unit:  "metered/0",
	}, {
		Key:   "juju-units",
		Value: "8",
		Time:  t1,
		Unit:  "metered/1",
	}, {
		Key:   "pings",
		Value: "5",
		Time:  t1,
		Unit:  "metered/1",
	}},
	)
}

func (s *metricsDebugSuite) TestGetMetricsFiltersCorrectlyWhenNotAllMetricsInEachBatch(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	unit0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	t0 := time.Now().Round(time.Second)
	t1 := t0.Add(time.Second)
	metricA := state.Metric{Key: "pings", Value: "5", Time: t1}
	metricB := state.Metric{Key: "pings", Value: "10.5", Time: t0}
	metricC := state.Metric{Key: "juju-units", Value: "8", Time: t1}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit0, Metrics: []state.Metric{metricA, metricB, metricC}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit1, Metrics: []state.Metric{metricA, metricB}})
	args := params.Entities{}
	result, err := s.metricsdebug.GetMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Metrics, gc.HasLen, 3)
	c.Assert(result.Results[0].Metrics, gc.DeepEquals, []params.MetricResult{{
		Key:   "juju-units",
		Value: "8",
		Time:  t1,
		Unit:  "metered/0",
	}, {
		Key:   "pings",
		Value: "5",
		Time:  t1,
		Unit:  "metered/0",
	}, {
		Key:   "pings",
		Value: "5",
		Time:  t1,
		Unit:  "metered/1",
	}},
	)
}

func (s *metricsDebugSuite) TestGetMetricsFiltersCorrectlyWithMultipleBatchesPerUnit(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	unit0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	t0 := time.Now().Round(time.Second)
	t1 := t0.Add(time.Second)
	metricA := state.Metric{Key: "pings", Value: "5", Time: t1}
	metricB := state.Metric{Key: "pings", Value: "10.5", Time: t0}
	metricC := state.Metric{Key: "juju-units", Value: "8", Time: t1}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit0, Metrics: []state.Metric{metricA, metricB}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit0, Metrics: []state.Metric{metricC}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit1, Metrics: []state.Metric{metricA, metricB}})
	args := params.Entities{}
	result, err := s.metricsdebug.GetMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Metrics, gc.HasLen, 3)
	c.Assert(result.Results[0].Metrics, jc.DeepEquals, []params.MetricResult{{
		Key:   "juju-units",
		Value: "8",
		Time:  t1,
		Unit:  "metered/0",
	}, {
		Key:   "pings",
		Value: "5",
		Time:  t1,
		Unit:  "metered/0",
	}, {
		Key:   "pings",
		Value: "5",
		Time:  t1,
		Unit:  "metered/1",
	}},
	)
}

func (s *metricsDebugSuite) TestGetMultipleMetricsNoMocks(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: meteredCharm,
	})
	unit0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})

	metricUnit0 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit0,
	})
	metricUnit1 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit1,
	})

	args0 := params.Entities{Entities: []params.Entity{
		{"unit-metered/0"},
	}}
	args1 := params.Entities{Entities: []params.Entity{
		{"unit-metered/1"},
	}}

	metrics0, err := s.metricsdebug.GetMetrics(args0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics0.Results, gc.HasLen, 1)
	c.Assert(metrics0.Results[0].Metrics[0].Key, gc.Equals, metricUnit0.Metrics()[0].Key)
	c.Assert(metrics0.Results[0].Metrics[0].Value, gc.Equals, metricUnit0.Metrics()[0].Value)
	c.Assert(metrics0.Results[0].Metrics[0].Time, jc.TimeBetween(metricUnit0.Metrics()[0].Time, metricUnit0.Metrics()[0].Time))

	metrics1, err := s.metricsdebug.GetMetrics(args1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics1.Results, gc.HasLen, 1)
	c.Assert(metrics1.Results[0].Metrics[0].Key, gc.Equals, metricUnit1.Metrics()[0].Key)
	c.Assert(metrics1.Results[0].Metrics[0].Value, gc.Equals, metricUnit1.Metrics()[0].Value)
	c.Assert(metrics1.Results[0].Metrics[0].Time, jc.TimeBetween(metricUnit1.Metrics()[0].Time, metricUnit1.Metrics()[0].Time))
}

func (s *metricsDebugSuite) TestGetMultipleMetricsNoMocksWithService(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: meteredCharm,
	})
	unit0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})

	metricUnit0 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit0,
	})
	metricUnit1 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit1,
	})

	args := params.Entities{Entities: []params.Entity{
		{"application-metered"},
	}}

	metrics, err := s.metricsdebug.GetMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics.Results, gc.HasLen, 1)
	c.Assert(metrics.Results[0].Metrics, gc.HasLen, 2)
	c.Assert(metrics.Results[0].Metrics[0].Key, gc.Equals, metricUnit0.Metrics()[0].Key)
	c.Assert(metrics.Results[0].Metrics[0].Value, gc.Equals, metricUnit0.Metrics()[0].Value)
	c.Assert(metrics.Results[0].Metrics[0].Time, jc.TimeBetween(metricUnit0.Metrics()[0].Time, metricUnit0.Metrics()[0].Time))

	c.Assert(metrics.Results[0].Metrics[1].Key, gc.Equals, metricUnit1.Metrics()[0].Key)
	c.Assert(metrics.Results[0].Metrics[1].Value, gc.Equals, metricUnit1.Metrics()[0].Value)
	c.Assert(metrics.Results[0].Metrics[1].Time, jc.TimeBetween(metricUnit1.Metrics()[0].Time, metricUnit1.Metrics()[0].Time))
}

func (s *metricsDebugSuite) TestGetModelNoMocks(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: meteredCharm,
	})
	unit0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})

	metricUnit0 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit0,
	})
	metricUnit1 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit1,
	})

	args := params.Entities{Entities: []params.Entity{}}
	metrics, err := s.metricsdebug.GetMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics.Results, gc.HasLen, 1)
	metric0 := metrics.Results[0].Metrics[0]
	metric1 := metrics.Results[0].Metrics[1]
	expected0 := metricUnit0.Metrics()[0]
	expected1 := metricUnit1.Metrics()[0]
	c.Assert(metric0.Key, gc.Equals, expected0.Key)
	c.Assert(metric0.Value, gc.Equals, expected0.Value)
	c.Assert(metric0.Time, jc.TimeBetween(expected0.Time, expected0.Time))
	c.Assert(metric0.Unit, gc.Equals, metricUnit0.Unit())
	c.Assert(metric1.Key, gc.Equals, expected1.Key)
	c.Assert(metric1.Value, gc.Equals, expected1.Value)
	c.Assert(metric1.Time, jc.TimeBetween(expected1.Time, expected1.Time))
	c.Assert(metric1.Unit, gc.Equals, metricUnit1.Unit())
}
