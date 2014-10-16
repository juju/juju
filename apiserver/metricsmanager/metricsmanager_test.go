// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/metricsender"
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
		Tag:            names.NewMachineTag("0"),
		EnvironManager: true,
	}
	manager, err := metricsmanager.NewMetricsManagerAPI(s.State, nil, s.authorizer)
	c.Assert(err, gc.IsNil)
	s.metricsmanager = manager
}

func (s *metricsManagerSuite) TestNewMetricsManagerAPIRefusesNonMachine(c *gc.C) {
	tests := []struct {
		tag            names.Tag
		environManager bool
		expectedError  string
	}{
		{names.NewUnitTag("mysql/0"), true, "permission denied"},
		{names.NewLocalUserTag("admin"), true, "permission denied"},
		{names.NewMachineTag("0"), false, "permission denied"},
		{names.NewMachineTag("0"), true, ""},
	}
	for i, test := range tests {
		c.Logf("test %d", i)

		anAuthoriser := s.authorizer
		anAuthoriser.EnvironManager = test.environManager
		anAuthoriser.Tag = test.tag
		endPoint, err := metricsmanager.NewMetricsManagerAPI(s.State, nil, anAuthoriser)
		if test.expectedError == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(endPoint, gc.NotNil)
		} else {
			c.Assert(err, gc.ErrorMatches, test.expectedError)
			c.Assert(endPoint, gc.IsNil)
		}
	}
}

func (s *metricsManagerSuite) TestCleanupOldMetrics(c *gc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	oldTime := time.Now().Add(-(time.Hour * 25))
	newTime := time.Now()
	oldMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &oldTime})
	newMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &newTime})
	args := params.Entities{Entities: []params.Entity{
		{s.State.EnvironTag().String()},
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
		{"invalid"},
	}}
	result, err := s.metricsmanager.CleanupOldMetrics(args)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(err, gc.IsNil)
	expectedError := common.ServerError(common.ErrPerm)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: expectedError})
}

func (s *metricsManagerSuite) TestCleanupArgsIndependent(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{"invalid"},
		{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsmanager.CleanupOldMetrics(args)
	c.Assert(result.Results, gc.HasLen, 2)
	c.Assert(err, gc.IsNil)
	expectedError := common.ServerError(common.ErrPerm)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: expectedError})
	c.Assert(result.Results[1], gc.DeepEquals, params.ErrorResult{Error: nil})
}

func (s *metricsManagerSuite) TestSendMetrics(c *gc.C) {
	var sender metricsender.MockSender
	metricsmanager.PatchSender(&sender)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now()
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &now})
	unsent := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	args := params.Entities{Entities: []params.Entity{
		{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
	c.Assert(sender.Data, gc.HasLen, 1)
	m, err := s.State.MetricBatch(unsent.UUID())
	c.Assert(err, gc.IsNil)
	c.Assert(m.Sent(), jc.IsTrue)
}

func (s *metricsManagerSuite) TestSendOldMetricsInvalidArg(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{"invalid"},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(err, gc.IsNil)
	expectedError := common.ServerError(common.ErrPerm)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: expectedError})
}

func (s *metricsManagerSuite) TestSendArgsIndependent(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{"invalid"},
		{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(result.Results, gc.HasLen, 2)
	c.Assert(err, gc.IsNil)
	expectedError := common.ServerError(common.ErrPerm)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: expectedError})
	c.Assert(result.Results[1], gc.DeepEquals, params.ErrorResult{Error: nil})
}
