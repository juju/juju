// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsadder_test

import (
	"time"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/metricsadder"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type metricsAdderSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&metricsAdderSuite{})

func (s *metricsAdderSuite) TestAddMetricBatches(c *gc.C) {
	uuid := utils.MustNewUUID().String()
	uuid2 := utils.MustNewUUID().String()
	batches := []params.MetricBatchParam{{
		Tag: names.NewUnitTag("test-unit/0").String(),
		Batch: params.MetricBatch{
			UUID:     uuid,
			CharmURL: "test-charm-url",
			Created:  time.Now(),
			Metrics:  []params.Metric{{Key: "pings", Value: "5", Time: time.Now().UTC()}},
		},
	}, {
		Tag: names.NewUnitTag("test-unit/0").String(),
		Batch: params.MetricBatch{
			UUID:     uuid2,
			CharmURL: "test-charm-url",
			Created:  time.Now(),
			Metrics:  []params.Metric{{Key: "pings", Value: "5", Time: time.Now().UTC()}},
		},
	}}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "MetricsAdder")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "AddMetricBatches")
		c.Check(arg, jc.DeepEquals, params.MetricBatchParams{
			Batches: batches,
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}, {}},
		}
		return nil
	})

	client := metricsadder.NewClient(apiCaller)
	result, err := client.AddMetricBatches(batches)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, map[string]error{
		uuid:  &params.Error{Message: "FAIL"},
		uuid2: (*params.Error)(nil),
	})
}
