// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/metricsmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type metricsManagerSuite struct {
	jujutesting.JujuConnSuite

	metricsmanager *metricsmanager.MetricsManagerAPI
	authorizer     apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&metricsManagerSuite{})

func (s *metricsManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	manager, err := metricsmanager.NewMetricsManagerAPI(s.State, nil, s.authorizer)
	c.Assert(err, gc.IsNil)
	s.metricsmanager = manager
}

func (s *metricsManagerSuite) TestCleanupOldMetrics(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	oldTime := time.Now().Add(-(time.Hour * 25))
	newTime := time.Now()
	oldMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &oldTime})
	newMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &newTime})
	args := params.Entities{Entities: []params.Entity{
		params.Entity{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsmanager.CleanupOldMetrics(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
	_, err = s.State.MetricBatch(oldMetric.UUID())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.State.MetricBatch(newMetric.UUID())
	c.Assert(err, gc.IsNil)
}

func (s *metricsManagerSuite) TestCleanupOldMetricsInvalidArg(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		params.Entity{"invalid"},
	}}
	result, err := s.metricsmanager.CleanupOldMetrics(args)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(err, gc.IsNil)
	expectedError := common.ServerError(common.ErrPerm)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: expectedError})
}

func (s *metricsManagerSuite) TestNewMetricsManagerAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authorizer
	anAuthoriser.Tag = names.NewUnitTag("mysql/0")
	endPoint, err := metricsmanager.NewMetricsManagerAPI(s.State, nil, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *metricsManagerSuite) TestCleanupArgsIndependant(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		params.Entity{"invalid"},
		params.Entity{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsmanager.CleanupOldMetrics(args)
	c.Assert(result.Results, gc.HasLen, 2)
	c.Assert(err, gc.IsNil)
	expectedError := common.ServerError(common.ErrPerm)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: expectedError})
	c.Assert(result.Results[1], gc.DeepEquals, params.ErrorResult{Error: nil})
}
