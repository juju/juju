// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"time"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/metricsdebug"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type cmdMetricsCommandSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&cmdMetricsCommandSuite{})

func (s *cmdMetricsCommandSuite) TestDebugNoArgs(c *gc.C) {
	_, err := coretesting.RunCommand(c, metricsdebug.New())
	c.Assert(err, gc.ErrorMatches, "you need to specify at least one unit or application")
}

func (s *cmdMetricsCommandSuite) TestUnits(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	newTime1 := time.Now().Round(time.Second)
	newTime2 := newTime1.Add(time.Second)
	metricA := state.Metric{"pings", "5", newTime1}
	metricB := state.Metric{"pings", "10.5", newTime2}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit2, Metrics: []state.Metric{metricA, metricB}})
	outputTime1 := newTime1.Format(time.RFC3339)
	outputTime2 := newTime2.Format(time.RFC3339)
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, fmt.Sprintf(`UNIT     	                TIMESTAMP	METRIC	VALUE
metered/1	%v	 pings	 10.5
`, outputTime2))
	ctx, err = coretesting.RunCommand(c, metricsdebug.New(), "metered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, fmt.Sprintf(`UNIT     	                TIMESTAMP	METRIC	VALUE
metered/0	%v	 pings	    5
`, outputTime1))
}

func (s *cmdMetricsCommandSuite) TestFormatJSON(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	newTime1 := time.Now().Round(time.Second)
	newTime2 := newTime1.Add(time.Second)
	metricA := state.Metric{"pings", "5", newTime1}
	metricB := state.Metric{"pings", "10.5", newTime2}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit2, Metrics: []state.Metric{metricA, metricB}})
	outputTime2 := newTime2.Format(time.RFC3339)
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered/1", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, fmt.Sprintf(`[{"unit":"metered/1","timestamp":"%v","metric":"pings","value":"10.5"}]
`, outputTime2))
}

func (s *cmdMetricsCommandSuite) TestFormatYAML(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	newTime1 := time.Now().Round(time.Second)
	newTime2 := newTime1.Add(time.Second)
	metricA := state.Metric{"pings", "5", newTime1}
	metricB := state.Metric{"pings", "10.5", newTime2}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit2, Metrics: []state.Metric{metricA, metricB}})
	outputTime2 := newTime2.Format(time.RFC3339)
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered/1", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, fmt.Sprintf(`- unit: metered/1
  timestamp: %v
  metric: pings
  value: "10.5"
`, outputTime2))
}

func (s *cmdMetricsCommandSuite) TestNoMetrics(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}
