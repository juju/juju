// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/metricsdebug"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type metricsdebugSuite struct {
	jujutesting.JujuConnSuite

	manager *metricsdebug.Client
}

var _ = gc.Suite(&metricsdebugSuite{})

func (s *metricsdebugSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.manager = metricsdebug.NewClient(s.APIState)
	c.Assert(s.manager, gc.NotNil)
}

func (s *metricsdebugSuite) TestGetMetrics(c *gc.C) {
	var called bool
	now := time.Now()
	metricsdebug.PatchFacadeCall(s, s.manager, func(request string, args, response interface{}) error {
		c.Assert(request, gc.Equals, "GetMetrics")
		result := response.(*params.MetricsResults)
		result.Results = []params.MetricsResult{{
			Metrics: []params.MetricResult{{
				Key:   "pings",
				Value: "5",
				Time:  now,
			}},
			Error: nil,
		}}
		called = true
		return nil
	})
	metrics, err := s.manager.GetMetrics("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Key, gc.Equals, "pings")
	c.Assert(metrics[0].Value, gc.Equals, "5")
	c.Assert(metrics[0].Time, gc.Equals, now)
}

func (s *metricsdebugSuite) TestGetMetricsFails(c *gc.C) {
	var called bool
	metricsdebug.PatchFacadeCall(s, s.manager, func(request string, args, response interface{}) error {
		c.Assert(request, gc.Equals, "GetMetrics")
		result := response.(*params.MetricsResults)
		result.Results = []params.MetricsResult{{
			Error: common.ServerError(errors.New("an error")),
		}}
		called = true
		return nil
	})
	_, err := s.manager.GetMetrics("wordpress/0")
	c.Assert(err, gc.ErrorMatches, "an error")
	c.Assert(called, jc.IsTrue)
}

func (s *metricsdebugSuite) TestGetMetricsFacadeCallError(c *gc.C) {
	var called bool
	metricsdebug.PatchFacadeCall(s, s.manager, func(request string, args, response interface{}) error {
		called = true
		return errors.New("an error")
	})
	_, err := s.manager.GetMetrics("wordpress/0")
	c.Assert(err, gc.ErrorMatches, "an error")
	c.Assert(called, jc.IsTrue)
}

func (s *metricsdebugSuite) TestGetMetricsNoMocks(c *gc.C) {
	metric := s.Factory.MakeMetric(c, nil)
	metrics, err := s.manager.GetMetrics("metered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Key, gc.Equals, metric.Metrics()[0].Key)
	c.Assert(metrics[0].Value, gc.Equals, metric.Metrics()[0].Value)
	c.Assert(metrics[0].Time, jc.TimeBetween(metric.Metrics()[0].Time, metric.Metrics()[0].Time))
}

func (s *metricsdebugSuite) TestGetMultipleMetricsNoMocks(c *gc.C) {
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

	metrics0, err := s.manager.GetMetrics("metered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics0, gc.HasLen, 1)
	c.Assert(metrics0[0].Key, gc.Equals, metricUnit0.Metrics()[0].Key)
	c.Assert(metrics0[0].Value, gc.Equals, metricUnit0.Metrics()[0].Value)
	c.Assert(metrics0[0].Time, jc.TimeBetween(metricUnit0.Metrics()[0].Time, metricUnit0.Metrics()[0].Time))

	metrics1, err := s.manager.GetMetrics("metered/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics1, gc.HasLen, 1)
	c.Assert(metrics1[0].Key, gc.Equals, metricUnit1.Metrics()[0].Key)
	c.Assert(metrics1[0].Value, gc.Equals, metricUnit1.Metrics()[0].Value)
	c.Assert(metrics1[0].Time, jc.TimeBetween(metricUnit1.Metrics()[0].Time, metricUnit1.Metrics()[0].Time))
}

func (s *metricsdebugSuite) TestGetMultipleMetricsNoMocksWithService(c *gc.C) {
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

	metrics, err := s.manager.GetMetrics("metered")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 2)
	c.Assert(metrics[0].Key, gc.Equals, metricUnit0.Metrics()[0].Key)
	c.Assert(metrics[0].Value, gc.Equals, metricUnit0.Metrics()[0].Value)
	c.Assert(metrics[0].Time, jc.TimeBetween(metricUnit0.Metrics()[0].Time, metricUnit0.Metrics()[0].Time))

	c.Assert(metrics[1].Key, gc.Equals, metricUnit1.Metrics()[0].Key)
	c.Assert(metrics[1].Value, gc.Equals, metricUnit1.Metrics()[0].Value)
	c.Assert(metrics[1].Time, jc.TimeBetween(metricUnit1.Metrics()[0].Time, metricUnit1.Metrics()[0].Time))
}
