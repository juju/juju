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
	"github.com/juju/juju/apiserver/metricsender/testing"
	"github.com/juju/juju/apiserver/metricsmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type metricsManagerSuite struct {
	jujutesting.JujuConnSuite

	metricsmanager *metricsmanager.MetricsManagerAPI
	authorizer     apiservertesting.FakeAuthorizer
	unit           *state.Unit
}

var _ = gc.Suite(&metricsManagerSuite{})

func (s *metricsManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("0"),
		EnvironManager: true,
	}
	manager, err := metricsmanager.NewMetricsManagerAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.metricsmanager = manager
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	s.unit = s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
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
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(endPoint, gc.NotNil)
		} else {
			c.Assert(err, gc.ErrorMatches, test.expectedError)
			c.Assert(endPoint, gc.IsNil)
		}
	}
}

func (s *metricsManagerSuite) TestCleanupOldMetrics(c *gc.C) {
	oldTime := time.Now().Add(-(time.Hour * 25))
	newTime := time.Now()
	metric := state.Metric{"pings", "5", newTime}
	oldMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, Time: &oldTime, Metrics: []state.Metric{metric}})
	newMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, Time: &newTime, Metrics: []state.Metric{metric}})
	args := params.Entities{Entities: []params.Entity{
		{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsmanager.CleanupOldMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
	_, err = s.State.MetricBatch(oldMetric.UUID())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.State.MetricBatch(newMetric.UUID())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *metricsManagerSuite) TestCleanupOldMetricsInvalidArg(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{"invalid"},
	}}
	result, err := s.metricsmanager.CleanupOldMetrics(args)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	expectedError := common.ServerError(common.ErrPerm)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: expectedError})
	c.Assert(result.Results[1], gc.DeepEquals, params.ErrorResult{Error: nil})
}

func (s *metricsManagerSuite) TestSendMetrics(c *gc.C) {
	var sender testing.MockSender
	metricsmanager.PatchSender(&sender)
	now := time.Now()
	metric := state.Metric{"pings", "5", now}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, Time: &now, Metrics: []state.Metric{metric}})
	unsent := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: []state.Metric{metric}})
	args := params.Entities{Entities: []params.Entity{
		{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
	c.Assert(sender.Data, gc.HasLen, 1)
	m, err := s.State.MetricBatch(unsent.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Sent(), jc.IsTrue)
}

func (s *metricsManagerSuite) TestSendOldMetricsInvalidArg(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{"invalid"},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(err, jc.ErrorIsNil)
	expectedError := `"invalid" is not a valid tag`
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedError)
}

func (s *metricsManagerSuite) TestSendArgsIndependent(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{"invalid"},
		{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(result.Results, gc.HasLen, 2)
	c.Assert(err, jc.ErrorIsNil)
	expectedError := `"invalid" is not a valid tag`
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedError)
	c.Assert(result.Results[1].Error, gc.IsNil)
}

func (s *metricsManagerSuite) TestMeterStatusOnConsecutiveErrors(c *gc.C) {
	var sender testing.ErrorSender
	sender.Err = errors.New("an error")
	now := time.Now()
	metric := state.Metric{"pings", "5", now}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: []state.Metric{metric}})
	metricsmanager.PatchSender(&sender)
	args := params.Entities{Entities: []params.Entity{
		{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedError := params.ErrorResult{Error: apiservertesting.PrefixedError("failed to send metrics: ", "an error")}
	c.Assert(result.Results[0], jc.DeepEquals, expectedError)
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.ConsecutiveErrors(), gc.Equals, 1)
}

func (s *metricsManagerSuite) TestMeterStatusSuccessfulSend(c *gc.C) {
	var sender testing.MockSender
	pastTime := time.Now().Add(-time.Second)
	metric := state.Metric{"pings", "5", pastTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &pastTime, Metrics: []state.Metric{metric}})
	metricsmanager.PatchSender(&sender)
	args := params.Entities{Entities: []params.Entity{
		{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.LastSuccessfulSend().After(pastTime), jc.IsTrue)
}

func (s *metricsManagerSuite) TestLastSuccessfulNotChangedIfNothingToSend(c *gc.C) {
	var sender testing.MockSender
	metricsmanager.PatchSender(&sender)
	args := params.Entities{Entities: []params.Entity{
		{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.LastSuccessfulSend().Equal(time.Time{}), jc.IsTrue)
}
