// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/metricsdebug"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type DebugMetricsCommandSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&DebugMetricsCommandSuite{})

func (s *DebugMetricsCommandSuite) TestDebugNoArgs(c *gc.C) {
	_, err := testing.RunCommand(c, metricsdebug.New())
	c.Assert(err, gc.ErrorMatches, "you need to specify a unit or service.")
}

func (s *DebugMetricsCommandSuite) TestUnit(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	newTime := time.Now().Round(time.Second)
	metricA := state.Metric{"pings", "5", newTime}
	metricB := state.Metric{"pings", "10.5", newTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA, metricB}})
	outputTime := newTime.Format(time.RFC3339)
	expectedOutput := fmt.Sprintf(`TIME                 METRIC VALUE
%v pings  5
%v pings  5
%v pings  10.5
`, outputTime, outputTime, outputTime)
	ctx, err := testing.RunCommand(c, metricsdebug.New(), "metered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedOutput)
}

func (s *DebugMetricsCommandSuite) TestUnits(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	newTime := time.Now().Round(time.Second)
	metricA := state.Metric{"pings", "5", newTime}
	metricB := state.Metric{"pings", "10.5", newTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit2, Metrics: []state.Metric{metricA, metricB}})
	outputTime := newTime.Format(time.RFC3339)
	expectedOutput := fmt.Sprintf(`TIME                 METRIC VALUE
%v pings  5
%v pings  10.5
`, outputTime, outputTime)
	ctx, err := testing.RunCommand(c, metricsdebug.New(), "metered/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedOutput)
}

func (s *DebugMetricsCommandSuite) TestService(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	newTime := time.Now().Round(time.Second)
	metricA := state.Metric{"pings", "5", newTime}
	metricB := state.Metric{"pings", "10.5", newTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit2, Metrics: []state.Metric{metricA, metricB}})
	outputTime := newTime.Format(time.RFC3339)
	expectedOutput := fmt.Sprintf(`TIME                 METRIC VALUE
%v pings  5
%v pings  5
%v pings  10.5
`, outputTime, outputTime, outputTime)
	ctx, err := testing.RunCommand(c, metricsdebug.New(), "metered")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedOutput)
}

func (s *DebugMetricsCommandSuite) TestServiceWithNoption(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	newTime := time.Now().Round(time.Second)
	metricA := state.Metric{"pings", "5", newTime}
	metricB := state.Metric{"pings", "10.5", newTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit2, Metrics: []state.Metric{metricA, metricB}})
	outputTime := newTime.Format(time.RFC3339)
	expectedOutput := fmt.Sprintf(`TIME                 METRIC VALUE
%v pings  5
%v pings  5
`, outputTime, outputTime)
	ctx, err := testing.RunCommand(c, metricsdebug.New(), "metered", "-n", "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedOutput)
}

func (s *DebugMetricsCommandSuite) TestServiceWithNoptionGreaterThanMetricCount(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	newTime := time.Now().Round(time.Second)
	metricA := state.Metric{"pings", "5", newTime}
	metricB := state.Metric{"pings", "10.5", newTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit2, Metrics: []state.Metric{metricA, metricB}})
	outputTime := newTime.Format(time.RFC3339)
	expectedOutput := fmt.Sprintf(`TIME                 METRIC VALUE
%v pings  5
%v pings  5
%v pings  10.5
`, outputTime, outputTime, outputTime)
	ctx, err := testing.RunCommand(c, metricsdebug.New(), "metered", "-n", "42")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedOutput)
}

func (s *DebugMetricsCommandSuite) TestNoMetrics(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	ctx, err := testing.RunCommand(c, metricsdebug.New(), "metered", "-n", "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *DebugMetricsCommandSuite) TestUnitJsonOutput(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	newTime := time.Now().Round(time.Second)
	metricA := state.Metric{"pings", "5", newTime}
	metricB := state.Metric{"pings", "10.5", newTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA, metricB}})
	outputTime := newTime.Format(time.RFC3339)
	expectedOutput := fmt.Sprintf(`[
    {
        "time": "%v",
        "key": "pings",
        "value": "5"
    },
    {
        "time": "%v",
        "key": "pings",
        "value": "5"
    },
    {
        "time": "%v",
        "key": "pings",
        "value": "10.5"
    }
]`, outputTime, outputTime, outputTime)
	ctx, err := testing.RunCommand(c, metricsdebug.New(), "metered/0", "--json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedOutput)
}
