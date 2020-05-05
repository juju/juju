// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager_test

import (
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/metricsender/testing"
	"github.com/juju/juju/apiserver/facades/controller/metricsmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type metricsManagerSuite struct {
	jujujutesting.JujuConnSuite

	clock          *testclock.Clock
	metricsmanager *metricsmanager.MetricsManagerAPI
	authorizer     apiservertesting.FakeAuthorizer
	unit           *state.Unit
}

var _ = gc.Suite(&metricsManagerSuite{})

func (s *metricsManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.clock = testclock.NewClock(time.Now())
	manager, err := metricsmanager.NewMetricsManagerAPI(s.State, nil, s.authorizer, s.StatePool, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	s.metricsmanager = manager
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredApplication := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	s.unit = s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredApplication, SetCharmURL: true})
}

func (s *metricsManagerSuite) TestNewMetricsManagerAPIRefusesNonController(c *gc.C) {
	tests := []struct {
		tag           names.Tag
		controller    bool
		expectedError string
	}{
		{names.NewUnitTag("mysql/0"), false, "permission denied"},
		{names.NewLocalUserTag("admin"), false, "permission denied"},
		{names.NewMachineTag("0"), false, "permission denied"},
		{names.NewMachineTag("0"), true, ""},
	}
	for i, test := range tests {
		c.Logf("test %d", i)

		anAuthoriser := s.authorizer
		anAuthoriser.Controller = test.controller
		anAuthoriser.Tag = test.tag
		endPoint, err := metricsmanager.NewMetricsManagerAPI(s.State, nil,
			anAuthoriser, s.StatePool, testclock.NewClock(time.Now()))
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
	metric := state.Metric{Key: "pings", Value: "5", Time: newTime}
	oldMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, DeleteTime: &oldTime, Metrics: []state.Metric{metric}})
	newMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, DeleteTime: &newTime, Metrics: []state.Metric{metric}})
	args := params.Entities{Entities: []params.Entity{
		{s.Model.ModelTag().String()},
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
		{s.Model.ModelTag().String()},
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
	cleanup := s.metricsmanager.PatchSender(&sender)
	defer cleanup()
	now := time.Now()
	metric := state.Metric{Key: "pings", Value: "5", Time: now}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, Time: &now, Metrics: []state.Metric{metric}})
	unsent := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: []state.Metric{metric}})
	args := params.Entities{Entities: []params.Entity{
		{s.Model.ModelTag().String()},
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
		{s.Model.ModelTag().String()},
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
	metric := state.Metric{Key: "pings", Value: "5", Time: now}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: []state.Metric{metric}})
	cleanup := s.metricsmanager.PatchSender(&sender)
	defer cleanup()
	args := params.Entities{Entities: []params.Entity{
		{s.Model.ModelTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedError := params.ErrorResult{Error: apiservertesting.PrefixedError(
		fmt.Sprintf("failed to send metrics for %s: ", s.Model.ModelTag()),
		"an error")}
	c.Assert(result.Results[0], jc.DeepEquals, expectedError)
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.ConsecutiveErrors(), gc.Equals, 1)
}

func (s *metricsManagerSuite) TestMeterStatusSuccessfulSend(c *gc.C) {
	var sender testing.MockSender
	pastTime := s.clock.Now().Add(-time.Second)
	metric := state.Metric{Key: "pings", Value: "5", Time: pastTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &pastTime, Metrics: []state.Metric{metric}})
	cleanup := s.metricsmanager.PatchSender(&sender)
	defer cleanup()
	args := params.Entities{Entities: []params.Entity{
		{s.Model.ModelTag().String()},
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
	cleanup := s.metricsmanager.PatchSender(&sender)
	defer cleanup()
	args := params.Entities{Entities: []params.Entity{
		{s.Model.ModelTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.LastSuccessfulSend().Equal(time.Time{}), jc.IsTrue)
}

func (s *metricsManagerSuite) TestAddJujuMachineMetrics(c *gc.C) {
	err := s.State.SetSLA("essential", "bob", []byte("sla"))
	c.Assert(err, jc.ErrorIsNil)
	// Create two additional ubuntu machines, in addition to the one created in setup.
	s.Factory.MakeMachine(c, &factory.MachineParams{Series: "trusty"})
	s.Factory.MakeMachine(c, &factory.MachineParams{Series: "xenial"})
	s.Factory.MakeMachine(c, &factory.MachineParams{Series: "win7"})
	s.Factory.MakeMachine(c, &factory.MachineParams{Series: "win8"})
	s.Factory.MakeMachine(c, &factory.MachineParams{Series: "centos7"})
	s.Factory.MakeMachine(c, &factory.MachineParams{Series: "redox"})
	err = s.metricsmanager.AddJujuMachineMetrics()
	c.Assert(err, jc.ErrorIsNil)
	metrics, err := s.State.MetricsToSend(10)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Metrics(), gc.HasLen, 5)
	c.Assert(metrics[0].SLACredentials(), gc.DeepEquals, []byte("sla"))
	t := metrics[0].Metrics()[0].Time
	c.Assert(metrics[0].UniqueMetrics(), jc.DeepEquals, []state.Metric{{
		Key:   "juju-centos-machines",
		Value: "1",
		Time:  t,
	}, {
		Key:   "juju-machines",
		Value: "7",
		Time:  t,
	}, {
		Key:   "juju-ubuntu-machines",
		Value: "3",
		Time:  t,
	}, {
		Key:   "juju-unknown-machines",
		Value: "1",
		Time:  t,
	}, {
		Key:   "juju-windows-machines",
		Value: "2",
		Time:  t,
	}})
}

func (s *metricsManagerSuite) TestAddJujuMachineMetricsAddsNoMetricsWhenNoSLASet(c *gc.C) {
	s.Factory.MakeMachine(c, nil)
	err := s.metricsmanager.AddJujuMachineMetrics()
	c.Assert(err, jc.ErrorIsNil)
	metrics, err := s.State.MetricsToSend(10)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 0)
}

func (s *metricsManagerSuite) TestAddJujuMachineMetricsDontCountContainers(c *gc.C) {
	err := s.State.SetSLA("essential", "bob", []byte("sla"))
	c.Assert(err, jc.ErrorIsNil)
	machine := s.Factory.MakeMachine(c, nil)
	s.Factory.MakeMachineNested(c, machine.Id(), nil)
	err = s.metricsmanager.AddJujuMachineMetrics()
	c.Assert(err, jc.ErrorIsNil)
	metrics, err := s.State.MetricsToSend(10)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Metrics()[0].Key, gc.Equals, "juju-machines")
	// Even though we add two machines - one nested (i.e. container) we only
	// count non-container machine.
	c.Assert(metrics[0].Metrics()[0].Value, gc.Equals, "2")
	c.Assert(metrics[0].SLACredentials(), gc.DeepEquals, []byte("sla"))
}

func (s *metricsManagerSuite) TestSendMetricsMachineMetrics(c *gc.C) {
	err := s.State.SetSLA("essential", "bob", []byte("sla"))
	c.Assert(err, jc.ErrorIsNil)
	s.Factory.MakeMachine(c, nil)
	var sender testing.MockSender
	cleanup := s.metricsmanager.PatchSender(&sender)
	defer cleanup()
	args := params.Entities{Entities: []params.Entity{
		{s.Model.ModelTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 1)
	c.Assert(sender.Data[0][0].Metrics[0].Key, gc.Equals, "juju-machines")
	c.Assert(sender.Data[0][0].SLACredentials, gc.DeepEquals, []byte("sla"))
	ms, err := s.State.AllMetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ms, gc.HasLen, 1)
	c.Assert(ms[0].Sent(), jc.IsTrue)
}
