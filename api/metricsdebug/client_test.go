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
