// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"errors"
	"time"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/metricsdebug"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type metricsdebugSuiteMock struct{}

var _ = gc.Suite(&metricsdebugSuiteMock{})

func (s *metricsdebugSuiteMock) TestGetMetrics(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	now := time.Now()
	args := params.Entities{
		Entities: []params.Entity{{Tag: "unit-wordpress/0"}},
	}
	res := new(params.MetricResults)
	ress := params.MetricResults{
		Results: []params.EntityMetrics{{
			Metrics: []params.MetricResult{{
				Key:    "pings",
				Value:  "5",
				Time:   now,
				Labels: map[string]string{"foo": "bar"},
			}},
			Error: nil,
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetMetrics", args, res).SetArg(2, ress).Return(nil)
	client := metricsdebug.NewClientFromCaller(mockFacadeCaller)

	metrics, err := client.GetMetrics("unit-wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Key, gc.Equals, "pings")
	c.Assert(metrics[0].Value, gc.Equals, "5")
	c.Assert(metrics[0].Time, gc.Equals, now)
	c.Assert(metrics[0].Labels, gc.HasLen, 1)
	c.Assert(metrics[0].Labels, gc.DeepEquals, map[string]string{"foo": "bar"})
}

func (s *metricsdebugSuiteMock) TestGetMetricsFails(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: "unit-wordpress/0"}},
	}
	res := new(params.MetricResults)
	ress := params.MetricResults{
		Results: []params.EntityMetrics{{
			Error: apiservererrors.ServerError(errors.New("an error")),
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetMetrics", args, res).SetArg(2, ress).Return(nil)
	client := metricsdebug.NewClientFromCaller(mockFacadeCaller)

	metrics, err := client.GetMetrics("unit-wordpress/0")
	c.Assert(err, gc.ErrorMatches, "an error")
	c.Assert(metrics, gc.IsNil)
}

func (s *metricsdebugSuiteMock) TestGetMetricsFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: "unit-wordpress/0"}},
	}
	res := new(params.MetricResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetMetrics", args, res).Return(errors.New("an error"))
	client := metricsdebug.NewClientFromCaller(mockFacadeCaller)

	metrics, err := client.GetMetrics("unit-wordpress/0")
	c.Assert(err, gc.ErrorMatches, "an error")
	c.Assert(metrics, gc.IsNil)
}

func (s *metricsdebugSuiteMock) TestGetMetricsForModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	now := time.Now()
	args := params.Entities{
		Entities: []params.Entity{},
	}
	res := new(params.MetricResults)
	ress := params.MetricResults{
		Results: []params.EntityMetrics{{
			Metrics: []params.MetricResult{{
				Key:   "pings",
				Value: "5",
				Time:  now,
			}},
			Error: nil,
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetMetrics", args, res).SetArg(2, ress).Return(nil)
	client := metricsdebug.NewClientFromCaller(mockFacadeCaller)

	metrics, err := client.GetMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Key, gc.Equals, "pings")
	c.Assert(metrics[0].Value, gc.Equals, "5")
	c.Assert(metrics[0].Time, gc.Equals, now)
}

func (s *metricsdebugSuiteMock) TestGetMetricsForModelFails(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{},
	}
	res := new(params.MetricResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetMetrics", args, res).Return(errors.New("an error"))
	client := metricsdebug.NewClientFromCaller(mockFacadeCaller)
	metrics, err := client.GetMetrics()
	c.Assert(metrics, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "an error")
}

func (s *metricsdebugSuiteMock) TestSetMeterStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.MeterStatusParams{
		Statuses: []params.MeterStatusParam{{
			Tag:  "unit-metered/0",
			Code: "RED",
			Info: "test"},
		},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SetMeterStatus", args, res).SetArg(2, ress).Return(nil)
	client := metricsdebug.NewClientFromCaller(mockFacadeCaller)
	err := client.SetMeterStatus("unit-metered/0", "RED", "test")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *metricsdebugSuiteMock) TestSetMeterStatusAPIServerError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.MeterStatusParams{
		Statuses: []params.MeterStatusParam{{
			Tag:  "unit-metered/0",
			Code: "RED",
			Info: "test"},
		},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: apiservererrors.ServerError(errors.New("an error")),
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SetMeterStatus", args, res).SetArg(2, ress).Return(nil)
	client := metricsdebug.NewClientFromCaller(mockFacadeCaller)
	err := client.SetMeterStatus("unit-metered/0", "RED", "test")
	c.Assert(err, gc.ErrorMatches, "an error")
}

func (s *metricsdebugSuiteMock) TestSetMeterStatusFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.MeterStatusParams{
		Statuses: []params.MeterStatusParam{{
			Tag:  "unit-metered/0",
			Code: "RED",
			Info: "test"},
		},
	}
	res := new(params.ErrorResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SetMeterStatus", args, res).Return(errors.New("an error"))
	client := metricsdebug.NewClientFromCaller(mockFacadeCaller)
	err := client.SetMeterStatus("unit-metered/0", "RED", "test")
	c.Assert(err, gc.ErrorMatches, "an error")
}

type metricsdebugSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&metricsdebugSuite{})

func assertSameMetric(c *gc.C, a params.MetricResult, b *state.MetricBatch) {
	c.Assert(a.Key, gc.Equals, b.Metrics()[0].Key)
	c.Assert(a.Value, gc.Equals, b.Metrics()[0].Value)
	c.Assert(a.Time, jc.TimeBetween(b.Metrics()[0].Time, b.Metrics()[0].Time))
}

func (s *metricsdebugSuite) TestFeatureGetMultipleMetrics(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	meteredApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: meteredCharm,
	})
	unit0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredApp, SetCharmURL: true})
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredApp, SetCharmURL: true})

	metricUnit0 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit0,
	})
	metricUnit1 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit1,
	})

	args0 := params.Entities{
		Entities: []params.Entity{{Tag: "unit-metered/0"}},
	}
	args1 := params.Entities{
		Entities: []params.Entity{{Tag: "unit-metered/1"}},
	}
	argsBoth := params.Entities{
		Entities: []params.Entity{{Tag: "unit-metered/0"}, {Tag: "unit-metered/1"}},
	}
	argsNone := params.Entities{
		Entities: []params.Entity{},
	}
	res := new(params.MetricResults)
	resMetric0 := metricUnit0.Metrics()[0]
	resMetric1 := metricUnit1.Metrics()[0]
	ress0 := params.MetricResults{
		Results: []params.EntityMetrics{{
			Metrics: []params.MetricResult{{
				Key:   resMetric0.Key,
				Value: resMetric0.Value,
				Time:  resMetric0.Time,
			}},
		}},
	}
	ress1 := params.MetricResults{
		Results: []params.EntityMetrics{{
			Metrics: []params.MetricResult{{
				Key:   resMetric1.Key,
				Value: resMetric1.Value,
				Time:  resMetric1.Time,
			}},
		}},
	}
	ressBoth := params.MetricResults{
		Results: []params.EntityMetrics{{
			Metrics: []params.MetricResult{{
				Key:   resMetric0.Key,
				Value: resMetric0.Value,
				Time:  resMetric0.Time,
			}, {
				Key:   resMetric1.Key,
				Value: resMetric1.Value,
				Time:  resMetric1.Time,
			}},
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetMetrics", args0, res).SetArg(2, ress0).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall("GetMetrics", args1, res).SetArg(2, ress1).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall("GetMetrics", argsBoth, res).SetArg(2, ressBoth).Return(nil)
	mockFacadeCaller.EXPECT().FacadeCall("GetMetrics", argsNone, res).SetArg(2, ressBoth).Return(nil)
	client := metricsdebug.NewClientFromCaller(mockFacadeCaller)

	metrics0, err := client.GetMetrics("unit-metered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics0, gc.HasLen, 1)
	assertSameMetric(c, metrics0[0], metricUnit0)

	metrics1, err := client.GetMetrics("unit-metered/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics1, gc.HasLen, 1)
	assertSameMetric(c, metrics1[0], metricUnit1)

	metrics2, err := client.GetMetrics("unit-metered/0", "unit-metered/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics2, gc.HasLen, 2)
	assertSameMetric(c, metrics2[0], metricUnit0)
	assertSameMetric(c, metrics2[1], metricUnit1)

	metrics3, err := client.GetMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics2, gc.HasLen, 2)
	assertSameMetric(c, metrics3[0], metricUnit0)
	assertSameMetric(c, metrics3[1], metricUnit1)
}

func (s *metricsdebugSuite) TestFeatureGetMultipleMetricsWithApplication(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	meteredApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: meteredCharm,
	})
	unit0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredApp, SetCharmURL: true})
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredApp, SetCharmURL: true})

	metricUnit0 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit0,
	})
	metricUnit1 := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit: unit1,
	})

	args := params.Entities{
		Entities: []params.Entity{{Tag: "application-metered"}},
	}
	res := new(params.MetricResults)
	resMetric0 := metricUnit0.Metrics()[0]
	resMetric1 := metricUnit1.Metrics()[0]
	ressBoth := params.MetricResults{
		Results: []params.EntityMetrics{{
			Metrics: []params.MetricResult{{
				Key:   resMetric0.Key,
				Value: resMetric0.Value,
				Time:  resMetric0.Time,
			}, {
				Key:   resMetric1.Key,
				Value: resMetric1.Value,
				Time:  resMetric1.Time,
			}},
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetMetrics", args, res).SetArg(2, ressBoth).Return(nil)
	client := metricsdebug.NewClientFromCaller(mockFacadeCaller)
	metrics, err := client.GetMetrics("application-metered")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 2)
	assertSameMetric(c, metrics[0], metricUnit0)
	assertSameMetric(c, metrics[1], metricUnit1)
}

func (s *metricsdebugSuite) TestSetMeterStatusMultiple(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	testCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered-1"})
	testApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: testCharm})
	testUnit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: testApp, SetCharmURL: true})
	testUnit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: testApp, SetCharmURL: true})

	csCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "ch:quantal/metered-1"})
	csApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "cs-service", Charm: csCharm})
	csUnit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: csApp, SetCharmURL: true})

	tests := []struct {
		about  string
		tag    string
		code   string
		info   string
		err    string
		assert func(*gc.C)
	}{{
		about: "set application meter status",
		tag:   testApp.Tag().String(),
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
		about: "not a local charm - application",
		tag:   csApp.Tag().String(),
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
		err:   "meter status \"NOT AVAILABLE\" not valid",
	}, {
		about: "no such application",
		tag:   "application-missing",
		code:  "AMBER",
		info:  "test",
		err:   "application \"missing\" not found",
	},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)

		args := params.MeterStatusParams{
			Statuses: []params.MeterStatusParam{{
				Tag:  test.tag,
				Code: test.code,
				Info: test.info},
			},
		}
		res := new(params.ErrorResults)
		var ress params.ErrorResults
		if test.err != "" {
			ress = params.ErrorResults{
				Results: []params.ErrorResult{{
					Error: &params.Error{Message: test.err},
				}},
			}
		} else {
			ress = params.ErrorResults{
				Results: []params.ErrorResult{{
					Error: nil,
				}},
			}
		}
		mockFacadeCaller.EXPECT().FacadeCall("SetMeterStatus", args, res).SetArg(2, ress).DoAndReturn(
			func(arg0 string, args params.MeterStatusParams, results *params.ErrorResults) []error {
				testUnit1.SetMeterStatus(test.code, test.info)
				testUnit2.SetMeterStatus(test.code, test.info)
				return nil
			})
		client := metricsdebug.NewClientFromCaller(mockFacadeCaller)
		err := client.SetMeterStatus(test.tag, test.code, test.info)
		if test.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			test.assert(c)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
		}
	}
}
