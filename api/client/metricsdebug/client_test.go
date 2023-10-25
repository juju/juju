// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/metricsdebug"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetMetrics", args, res).SetArg(3, ress).Return(nil)
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetMetrics", args, res).SetArg(3, ress).Return(nil)
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetMetrics", args, res).Return(errors.New("an error"))
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetMetrics", args, res).SetArg(3, ress).Return(nil)
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetMetrics", args, res).Return(errors.New("an error"))
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetMeterStatus", args, res).SetArg(3, ress).Return(nil)
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetMeterStatus", args, res).SetArg(3, ress).Return(nil)
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetMeterStatus", args, res).Return(errors.New("an error"))
	client := metricsdebug.NewClientFromCaller(mockFacadeCaller)
	err := client.SetMeterStatus("unit-metered/0", "RED", "test")
	c.Assert(err, gc.ErrorMatches, "an error")
}
