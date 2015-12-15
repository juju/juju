// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/metricsdebug"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type metricsManagerSuite struct {
	jujutesting.JujuConnSuite

	metricsdebug *metricsdebug.MetricsDebugAPI
	authorizer   apiservertesting.FakeAuthorizer
	unit         *state.Unit
}

var _ = gc.Suite(&metricsManagerSuite{})

func (s *metricsManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("0"),
		EnvironManager: true,
	}
	manager, err := metricsdebug.NewMetricsDebugAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.metricsdebug = manager
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	s.unit = s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
}

// TODO: What should we do instead?
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
		endPoint, err := metricsdebug.NewMetricsDebugAPI(s.State, nil, anAuthoriser)
		if test.expectedError == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(endPoint, gc.NotNil)
		} else {
			c.Assert(err, gc.ErrorMatches, test.expectedError)
			c.Assert(endPoint, gc.IsNil)
		}
	}
}

func (s *metricsManagerSuite) TestGetMetrics(c *gc.C) {
	newTime := time.Now().Round(time.Second)
	metricA := state.Metric{"pings", "5", newTime}
	metricB := state.Metric{"pings", "10.5", newTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Metrics: []state.Metric{metricA, metricB}})
	args := params.Entities{Entities: []params.Entity{
		{s.State.EnvironTag().String()},
	}}
	result, err := s.metricsdebug.GetMetrics(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 2)
	c.Assert(result.Results[0], gc.DeepEquals, params.MetricsResult{
		Metric: []params.MetricResult{
			{
				Key:   "pings",
				Value: "5",
				Time:  newTime,
			},
		},
		Error: nil,
	})
	c.Assert(result.Results[1], gc.DeepEquals, params.MetricsResult{
		Metric: []params.MetricResult{
			{
				Key:   "pings",
				Value: "5",
				Time:  newTime,
			},
			{
				Key:   "pings",
				Value: "10.5",
				Time:  newTime,
			},
		},
		Error: nil,
	})
}
