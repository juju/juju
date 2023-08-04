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

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/agent/metricsender/testing"
	"github.com/juju/juju/apiserver/facades/controller/metricsmanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type metricsManagerSuite struct {
	jujujutesting.ApiServerSuite

	clock          *testclock.Clock
	metricsmanager *metricsmanager.MetricsManagerAPI
	authorizer     apiservertesting.FakeAuthorizer
	unit           *state.Unit
}

var _ = gc.Suite(&metricsManagerSuite{})

func (s *metricsManagerSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.clock = testclock.NewClock(time.Now())
	manager, err := metricsmanager.NewMetricsManagerAPI(s.ControllerModel(c).State(), nil, s.authorizer, s.StatePool(), s.clock)
	c.Assert(err, jc.ErrorIsNil)
	s.metricsmanager = manager
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	meteredCharm := f.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "ch:amd64/quantal/metered"})
	meteredApplication := f.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	s.unit = f.MakeUnit(c, &factory.UnitParams{Application: meteredApplication, SetCharmURL: true})
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
		endPoint, err := metricsmanager.NewMetricsManagerAPI(s.ControllerModel(c).State(), nil,
			anAuthoriser, s.StatePool(), testclock.NewClock(time.Now()))
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
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	oldMetric := f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, DeleteTime: &oldTime, Metrics: []state.Metric{metric}})
	newMetric := f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, DeleteTime: &newTime, Metrics: []state.Metric{metric}})
	args := params.Entities{Entities: []params.Entity{
		{s.ControllerModel(c).ModelTag().String()},
	}}
	result, err := s.metricsmanager.CleanupOldMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
	st := s.ControllerModel(c).State()
	_, err = st.MetricBatch(oldMetric.UUID())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = st.MetricBatch(newMetric.UUID())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *metricsManagerSuite) TestCleanupOldMetricsInvalidArg(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{"invalid"},
	}}
	result, err := s.metricsmanager.CleanupOldMetrics(args)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(err, jc.ErrorIsNil)
	expectedError := apiservererrors.ServerError(apiservererrors.ErrPerm)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: expectedError})
}

func (s *metricsManagerSuite) TestCleanupArgsIndependent(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{"invalid"},
		{s.ControllerModel(c).ModelTag().String()},
	}}
	result, err := s.metricsmanager.CleanupOldMetrics(args)
	c.Assert(result.Results, gc.HasLen, 2)
	c.Assert(err, jc.ErrorIsNil)
	expectedError := apiservererrors.ServerError(apiservererrors.ErrPerm)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: expectedError})
	c.Assert(result.Results[1], gc.DeepEquals, params.ErrorResult{Error: nil})
}

func (s *metricsManagerSuite) TestSendMetrics(c *gc.C) {
	var sender testing.MockSender
	cleanup := s.metricsmanager.PatchSender(&sender)
	defer cleanup()
	now := time.Now()
	metric := state.Metric{Key: "pings", Value: "5", Time: now}
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: true, Time: &now, Metrics: []state.Metric{metric}})
	unsent := f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: []state.Metric{metric}})
	args := params.Entities{Entities: []params.Entity{
		{s.ControllerModel(c).ModelTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
	c.Assert(sender.Data, gc.HasLen, 1)
	st := s.ControllerModel(c).State()
	m, err := st.MetricBatch(unsent.UUID())
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
		{s.ControllerModel(c).ModelTag().String()},
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
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now, Metrics: []state.Metric{metric}})
	cleanup := s.metricsmanager.PatchSender(&sender)
	defer cleanup()
	args := params.Entities{Entities: []params.Entity{
		{s.ControllerModel(c).ModelTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedError := params.ErrorResult{Error: apiservertesting.PrefixedError(
		fmt.Sprintf("failed to send metrics for %s: ", s.ControllerModel(c).ModelTag()),
		"an error")}
	c.Assert(result.Results[0], jc.DeepEquals, expectedError)
	mm, err := s.ControllerModel(c).State().MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.ConsecutiveErrors(), gc.Equals, 1)
}

func (s *metricsManagerSuite) TestMeterStatusSuccessfulSend(c *gc.C) {
	var sender testing.MockSender
	pastTime := s.clock.Now().Add(-time.Second)
	metric := state.Metric{Key: "pings", Value: "5", Time: pastTime}
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &pastTime, Metrics: []state.Metric{metric}})
	cleanup := s.metricsmanager.PatchSender(&sender)
	defer cleanup()
	args := params.Entities{Entities: []params.Entity{
		{s.ControllerModel(c).ModelTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	mm, err := s.ControllerModel(c).State().MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.LastSuccessfulSend().After(pastTime), jc.IsTrue)
}

func (s *metricsManagerSuite) TestLastSuccessfulNotChangedIfNothingToSend(c *gc.C) {
	var sender testing.MockSender
	cleanup := s.metricsmanager.PatchSender(&sender)
	defer cleanup()
	args := params.Entities{Entities: []params.Entity{
		{s.ControllerModel(c).ModelTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	mm, err := s.ControllerModel(c).State().MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.LastSuccessfulSend().Equal(time.Time{}), jc.IsTrue)
}

func (s *metricsManagerSuite) TestAddJujuMachineMetrics(c *gc.C) {
	st := s.ControllerModel(c).State()
	err := st.SetSLA("essential", "bob", []byte("sla"))
	c.Assert(err, jc.ErrorIsNil)
	// Create two additional ubuntu machines, in addition to the one created in setup.
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeMachine(c, &factory.MachineParams{Base: state.UbuntuBase("22.04")})
	f.MakeMachine(c, &factory.MachineParams{Base: state.UbuntuBase("20.04")})
	f.MakeMachine(c, &factory.MachineParams{Base: state.Base{OS: "centos", Channel: "7"}})
	f.MakeMachine(c, &factory.MachineParams{Base: state.Base{OS: "zzzz", Channel: "redux"}})
	err = s.metricsmanager.AddJujuMachineMetrics()
	c.Assert(err, jc.ErrorIsNil)
	metrics, err := st.MetricsToSend(10)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Metrics(), gc.HasLen, 4)
	c.Assert(metrics[0].SLACredentials(), gc.DeepEquals, []byte("sla"))
	t := metrics[0].Metrics()[0].Time
	c.Assert(metrics[0].UniqueMetrics(), jc.DeepEquals, []state.Metric{{
		Key:   "juju-centos-machines",
		Value: "1",
		Time:  t,
	}, {
		Key:   "juju-machines",
		Value: "5",
		Time:  t,
	}, {
		Key:   "juju-ubuntu-machines",
		Value: "3",
		Time:  t,
	}, {
		Key:   "juju-zzzz-machines",
		Value: "1",
		Time:  t,
	}})
}

func (s *metricsManagerSuite) TestAddJujuMachineMetricsAddsNoMetricsWhenNoSLASet(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeMachine(c, nil)
	err := s.metricsmanager.AddJujuMachineMetrics()
	c.Assert(err, jc.ErrorIsNil)
	metrics, err := s.ControllerModel(c).State().MetricsToSend(10)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 0)
}

func (s *metricsManagerSuite) TestAddJujuMachineMetricsDontCountContainers(c *gc.C) {
	st := s.ControllerModel(c).State()
	err := st.SetSLA("essential", "bob", []byte("sla"))
	c.Assert(err, jc.ErrorIsNil)
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	machine := f.MakeMachine(c, nil)
	f.MakeMachineNested(c, machine.Id(), nil)
	err = s.metricsmanager.AddJujuMachineMetrics()
	c.Assert(err, jc.ErrorIsNil)
	metrics, err := st.MetricsToSend(10)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Metrics()[0].Key, gc.Equals, "juju-machines")
	// Even though we add two machines - one nested (i.e. container) we only
	// count non-container machine.
	c.Assert(metrics[0].Metrics()[0].Value, gc.Equals, "2")
	c.Assert(metrics[0].SLACredentials(), gc.DeepEquals, []byte("sla"))
}

func (s *metricsManagerSuite) TestSendMetricsMachineMetrics(c *gc.C) {
	st := s.ControllerModel(c).State()
	err := st.SetSLA("essential", "bob", []byte("sla"))
	c.Assert(err, jc.ErrorIsNil)
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeMachine(c, nil)
	var sender testing.MockSender
	cleanup := s.metricsmanager.PatchSender(&sender)
	defer cleanup()
	args := params.Entities{Entities: []params.Entity{
		{s.ControllerModel(c).ModelTag().String()},
	}}
	result, err := s.metricsmanager.SendMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
	c.Assert(sender.Data, gc.HasLen, 1)
	c.Assert(sender.Data[0], gc.HasLen, 1)
	c.Assert(sender.Data[0][0].Metrics[0].Key, gc.Equals, "juju-machines")
	c.Assert(sender.Data[0][0].SLACredentials, gc.DeepEquals, []byte("sla"))
	ms, err := st.AllMetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ms, gc.HasLen, 1)
	c.Assert(ms[0].Sent(), jc.IsTrue)
}
