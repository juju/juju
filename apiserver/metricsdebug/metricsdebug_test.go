// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/metricsdebug"
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

func (s *metricsDebugSuite) TestGetMetrics(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	newTime := time.Now().Round(time.Second)
	metricA := state.Metric{"pings", "5", newTime}
	metricB := state.Metric{"pings", "10.5", newTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA, metricB}})
	args := params.Entities{Entities: []params.Entity{
		{"unit-metered/0"},
	}}
	result, err := s.metricsdebug.GetMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Metrics, gc.HasLen, 3)
	c.Assert(result.Results[0], gc.DeepEquals, params.EntityMetrics{
		Metrics: []params.MetricResult{
			{
				Key:   "pings",
				Value: "5",
				Time:  newTime,
			},
			{
				Key:   "pings",
				Value: "5",
				Time:  newTime,
			},
			{
				Key:   "pings",
				Value: "10.5",
				Time:  newTime,
			},
		},
		Error: nil,
	})
}

func (s *metricsDebugSuite) TestGetMultipleMetricsNoMocks(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{
		Charm: meteredCharm,
	})
	unit0 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})

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
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{
		Charm: meteredCharm,
	})
	unit0 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})

	metricUnit0 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit0,
	})
	metricUnit1 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit1,
	})

	args := params.Entities{Entities: []params.Entity{
		{"service-metered"},
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
