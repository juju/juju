// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/metricsmanager"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
)

type metricsManagerSuite struct {
	jujutesting.JujuConnSuite

	manager *metricsmanager.Client
}

var _ = gc.Suite(&metricsManagerSuite{})

func (s *metricsManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.manager = metricsmanager.NewClient(s.APIState)
	c.Assert(s.manager, gc.NotNil)
}

func (s *metricsManagerSuite) TestCleanupOldMetrics(c *gc.C) {
	var called bool
	metricsmanager.PatchFacadeCall(s, s.manager, func(request string, args, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "CleanupOldMetrics")
		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})
	err := s.manager.CleanupOldMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *metricsManagerSuite) TestSendMetrics(c *gc.C) {
	var called bool
	metricsmanager.PatchFacadeCall(s, s.manager, func(request string, args, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "SendMetrics")
		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})
	err := s.manager.SendMetrics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *metricsManagerSuite) TestSendMetricsFails(c *gc.C) {
	var called bool
	metricsmanager.PatchFacadeCall(s, s.manager, func(request string, args, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "SendMetrics")
		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		result.Results[0].Error = common.ServerError(common.ErrPerm)
		return result.OneError()
	})
	err := s.manager.SendMetrics()
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(called, jc.IsTrue)
}
