// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/metricsmanager"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type metricsManagerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&metricsManagerSuite{})

func (s *metricsManagerSuite) TestCleanupOldMetrics(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "MetricsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "CleanupOldMetrics")
		c.Check(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: coretesting.ModelTag.String(),
		}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "FAIL"}}},
		}
		return nil
	})
	client, err := metricsmanager.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	err = client.CleanupOldMetrics()
	c.Assert(err, jc.DeepEquals, &params.Error{Message: "FAIL"})
}

func (s *metricsManagerSuite) TestSendMetrics(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "MetricsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SendMetrics")
		c.Check(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: coretesting.ModelTag.String(),
		}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "FAIL"}}},
		}
		return nil
	})
	client, err := metricsmanager.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	err = client.SendMetrics()
	c.Assert(err, jc.DeepEquals, &params.Error{Message: "FAIL"})
}
