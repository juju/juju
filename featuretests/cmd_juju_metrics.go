// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"time"

	"github.com/gosuri/uitable"
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

type metric struct {
	Unit      string    `json:"unit" yaml:"unit"`
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`
	Metric    string    `json:"metric" yaml:"metric"`
	Value     string    `json:"value" yaml:"value"`
}

func formatTabular(metrics ...metric) string {
	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true
	for _, col := range []int{1, 2, 3, 4} {
		table.RightAlign(col)
	}
	table.AddRow("UNIT", "TIMESTAMP", "METRIC", "VALUE")
	for _, m := range metrics {
		table.AddRow(m.Unit, m.Timestamp.Format(time.RFC3339), m.Metric, m.Value)
	}
	return table.String() + "\n"
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
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered/1")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cmdtesting.Stdout(ctx), gc.Equals,
		formatTabular(metric{
			Unit:      unit2.Name(),
			Timestamp: newTime2,
			Metric:    "pings",
			Value:     "10.5",
		}),
	)
	ctx, err = coretesting.RunCommand(c, metricsdebug.New(), "metered/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals,
		formatTabular(metric{
			Unit:      unit.Name(),
			Timestamp: newTime1,
			Metric:    "pings",
			Value:     "5",
		}),
	)
}

func (s *cmdMetricsCommandSuite) TestAll(c *gc.C) {
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
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "--all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals,
		formatTabular([]metric{{
			Unit:      unit.Name(),
			Timestamp: newTime1,
			Metric:    "pings",
			Value:     "5",
		}, {
			Unit:      unit2.Name(),
			Timestamp: newTime2,
			Metric:    "pings",
			Value:     "10.5",
		}}...),
	)
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
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered/1", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput := []metric{{
		Unit:      unit2.Name(),
		Timestamp: newTime2,
		Metric:    "pings",
		Value:     "10.5",
	}}
	c.Assert(cmdtesting.Stdout(ctx), jc.JSONEquals, expectedOutput)
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
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered/1", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	expectedOutput := []metric{{
		Unit:      unit2.Name(),
		Timestamp: newTime2,
		Metric:    "pings",
		Value:     "10.5",
	}}
	c.Assert(cmdtesting.Stdout(ctx), jc.YAMLEquals, expectedOutput)
}

func (s *cmdMetricsCommandSuite) TestNoMetrics(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	meteredService := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredService, SetCharmURL: true})
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}
