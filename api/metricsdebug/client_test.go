// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/metricsdebug"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type metricsdebugSuiteMock struct {
	testing.BaseSuite
	manager *metricsdebug.Client
}

var _ = gc.Suite(&metricsdebugSuiteMock{})

func (s *metricsdebugSuite) TestGetMetrics(c *gc.C) {
	var called bool
	now := time.Now()
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, response interface{},
		) error {
			c.Assert(request, gc.Equals, "GetMetrics")
			result := response.(*params.MetricResults)
			result.Results = []params.EntityMetrics{{
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
	client := metricsdebug.NewClient(apiCaller)
	metrics, err := client.GetMetrics("unit-wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Key, gc.Equals, "pings")
	c.Assert(metrics[0].Value, gc.Equals, "5")
	c.Assert(metrics[0].Time, gc.Equals, now)
}

func (s *metricsdebugSuiteMock) TestGetMetricsFails(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, response interface{},
		) error {
			c.Assert(request, gc.Equals, "GetMetrics")
			result := response.(*params.MetricResults)
			result.Results = []params.EntityMetrics{{
				Error: common.ServerError(errors.New("an error")),
			}}
			called = true
			return nil
		})
	client := metricsdebug.NewClient(apiCaller)
	metrics, err := client.GetMetrics("unit-wordpress/0")
	c.Assert(err, gc.ErrorMatches, "an error")
	c.Assert(metrics, gc.IsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *metricsdebugSuiteMock) TestGetMetricsFacadeCallError(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			return errors.New("an error")
		})
	client := metricsdebug.NewClient(apiCaller)
	metrics, err := client.GetMetrics("unit-wordpress/0")
	c.Assert(err, gc.ErrorMatches, "an error")
	c.Assert(metrics, gc.IsNil)
	c.Assert(called, jc.IsTrue)
}

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

func assertSameMetric(c *gc.C, a params.MetricResult, b *state.MetricBatch) {
	c.Assert(a.Key, gc.Equals, b.Metrics()[0].Key)
	c.Assert(a.Value, gc.Equals, b.Metrics()[0].Value)
	c.Assert(a.Time, jc.TimeBetween(b.Metrics()[0].Time, b.Metrics()[0].Time))
}

func (s *metricsdebugSuite) TestFeatureGetMetrics(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	metric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit})
	metrics, err := s.manager.GetMetrics("unit-metered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 1)
	assertSameMetric(c, metrics[0], metric)
}

func (s *metricsdebugSuite) TestFeatureGetMultipleMetrics(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
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

	metrics0, err := s.manager.GetMetrics("unit-metered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics0, gc.HasLen, 1)
	assertSameMetric(c, metrics0[0], metricUnit0)

	metrics1, err := s.manager.GetMetrics("unit-metered/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics1, gc.HasLen, 1)
	assertSameMetric(c, metrics1[0], metricUnit1)
}

func (s *metricsdebugSuite) TestFeatureGetMultipleMetricsWithService(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
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

	metrics, err := s.manager.GetMetrics("service-metered")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 2)
	assertSameMetric(c, metrics[0], metricUnit0)
	assertSameMetric(c, metrics[1], metricUnit1)
}
