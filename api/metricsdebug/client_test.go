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

func (s *metricsdebugSuiteMock) TestGetMetrics(c *gc.C) {
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

func (s *metricsdebugSuiteMock) TestSetMeterStatus(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, response interface{},
		) error {
			c.Assert(request, gc.Equals, "SetMeterStatus")
			c.Assert(a, gc.DeepEquals, params.MeterStatusParams{
				Statuses: []params.MeterStatusParam{{
					Tag:  "unit-metered/0",
					Code: "RED",
					Info: "test"},
				},
			})
			result := response.(*params.ErrorResults)
			result.Results = []params.ErrorResult{{
				Error: nil,
			}}
			called = true
			return nil
		})
	client := metricsdebug.NewClient(apiCaller)
	err := client.SetMeterStatus("unit-metered/0", "RED", "test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *metricsdebugSuiteMock) TestSetMeterStatusAPIServerError(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, response interface{},
		) error {
			c.Assert(request, gc.Equals, "SetMeterStatus")
			c.Assert(a, gc.DeepEquals, params.MeterStatusParams{
				Statuses: []params.MeterStatusParam{{
					Tag:  "unit-metered/0",
					Code: "RED",
					Info: "test"},
				},
			})
			result := response.(*params.ErrorResults)
			result.Results = []params.ErrorResult{{
				Error: common.ServerError(errors.New("an error")),
			}}
			called = true
			return nil
		})
	client := metricsdebug.NewClient(apiCaller)
	err := client.SetMeterStatus("unit-metered/0", "RED", "test")
	c.Assert(err, gc.ErrorMatches, "an error")
	c.Assert(called, jc.IsTrue)
}

func (s *metricsdebugSuiteMock) TestSetMeterStatusFacadeCallError(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, response interface{},
		) error {
			called = true
			return errors.New("an error")
		})
	client := metricsdebug.NewClient(apiCaller)
	err := client.SetMeterStatus("unit-metered/0", "RED", "test")
	c.Assert(err, gc.ErrorMatches, "an error")
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

func (s *metricsdebugSuite) TestSetMeterStatus(c *gc.C) {
	testCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	testService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: testCharm})
	testUnit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: testService, SetCharmURL: true})
	testUnit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: testService, SetCharmURL: true})

	csCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	csService := s.Factory.MakeService(c, &factory.ServiceParams{Name: "cs-service", Charm: csCharm})
	csUnit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: csService, SetCharmURL: true})

	tests := []struct {
		about  string
		tag    string
		code   string
		info   string
		err    string
		assert func(*gc.C)
	}{{
		about: "set service meter status",
		tag:   testService.Tag().String(),
		code:  "RED",
		info:  "test",
		assert: func(c *gc.C) {
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
		tag:   testUnit1.Tag().String(),
		code:  "AMBER",
		info:  "test",
		assert: func(c *gc.C) {
			ms1, err := testUnit1.GetMeterStatus()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(ms1, gc.DeepEquals, state.MeterStatus{
				Code: state.MeterAmber,
				Info: "test",
			})
		},
	}, {
		about: "not a local charm - service",
		tag:   csService.Tag().String(),
		code:  "AMBER",
		info:  "test",
		err:   "not a local charm",
	}, {
		about: "not a local charm - unit",
		tag:   csUnit1.Tag().String(),
		code:  "AMBER",
		info:  "test",
		err:   "not a local charm",
	}, {
		about: "invalid meter status",
		tag:   testUnit1.Tag().String(),
		code:  "WRONG",
		info:  "test",
		err:   "invalid meter status \"NOT AVAILABLE\"",
	}, {
		about: "not such service",
		tag:   "service-missing",
		code:  "AMBER",
		info:  "test",
		err:   "service \"missing\" not found",
	},
	}

	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		err := s.manager.SetMeterStatus(test.tag, test.code, test.info)
		if test.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			test.assert(c)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
		}
	}
}
