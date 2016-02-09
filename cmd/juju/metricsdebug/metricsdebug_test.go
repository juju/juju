// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"bytes"
	"fmt"
	stdtesting "testing"
	"text/tabwriter"
	"time"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/metricsdebug"
	"github.com/juju/juju/cmd/modelcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type MockGetMetricsClient struct {
	testing.Stub
}

func (m *MockGetMetricsClient) GetMetrics(tag string) ([]params.MetricResult, error) {
	m.Stub.MethodCall(m, "GetMetrics", tag)
	return nil, nil
}
func (m *MockGetMetricsClient) Close() error {
	m.Stub.MethodCall(m, "Close")
	return nil
}

type DebugMetricsMockSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&DebugMetricsMockSuite{})

func (s *DebugMetricsMockSuite) TestUnit(c *gc.C) {
	client := MockGetMetricsClient{testing.Stub{}}
	s.PatchValue(metricsdebug.NewClient, func(_ modelcmd.ModelCommandBase) (metricsdebug.GetMetricsClient, error) {
		return &client, nil
	})
	_, err := coretesting.RunCommand(c, metricsdebug.New(), "metered/0")
	c.Assert(err, jc.ErrorIsNil)
	client.CheckCall(c, 0, "GetMetrics", "unit-metered-0")
}

func (s *DebugMetricsMockSuite) TestService(c *gc.C) {
	client := MockGetMetricsClient{testing.Stub{}}
	s.PatchValue(metricsdebug.NewClient, func(_ modelcmd.ModelCommandBase) (metricsdebug.GetMetricsClient, error) {
		return &client, nil
	})
	_, err := coretesting.RunCommand(c, metricsdebug.New(), "metered")
	c.Assert(err, jc.ErrorIsNil)
	client.CheckCall(c, 0, "GetMetrics", "service-metered")
}

func (s *DebugMetricsMockSuite) TestNotValidServiceOrUnit(c *gc.C) {
	client := MockGetMetricsClient{testing.Stub{}}
	s.PatchValue(metricsdebug.NewClient, func(_ modelcmd.ModelCommandBase) (metricsdebug.GetMetricsClient, error) {
		return &client, nil
	})
	_, err := coretesting.RunCommand(c, metricsdebug.New(), "!!!!!!")
	c.Assert(err, gc.ErrorMatches, `"!!!!!!" is not a valid unit or service`)
}

type DebugMetricsCommandSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&DebugMetricsCommandSuite{})

func (s *DebugMetricsCommandSuite) TestDebugNoArgs(c *gc.C) {
	_, err := coretesting.RunCommand(c, metricsdebug.New())
	c.Assert(err, gc.ErrorMatches, "you need to specify a unit or service.")
}

func (s *DebugMetricsCommandSuite) TestUnits(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	newTime := time.Now().Round(time.Second)
	metricA := state.Metric{"pings", "5", newTime}
	metricB := state.Metric{"pings", "10.5", newTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit2, Metrics: []state.Metric{metricA, metricB}})
	outputTime := newTime.Format(time.RFC3339)
	expectedOutput := bytes.Buffer{}
	tw := tabwriter.NewWriter(&expectedOutput, 0, 1, 1, ' ', 0)
	fmt.Fprintf(tw, "TIME\tMETRIC\tVALUE\n")
	fmt.Fprintf(tw, "%v\tpings\t5\n", outputTime)
	fmt.Fprintf(tw, "%v\tpings\t10.5\n", outputTime)
	tw.Flush()
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedOutput.String())
}

func (s *DebugMetricsCommandSuite) TestServiceWithNoption(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	newTime := time.Now().Round(time.Second)
	metricA := state.Metric{"pings", "5", newTime}
	metricB := state.Metric{"pings", "10.5", newTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit2, Metrics: []state.Metric{metricA, metricB}})
	outputTime := newTime.Format(time.RFC3339)
	expectedOutput := bytes.Buffer{}
	tw := tabwriter.NewWriter(&expectedOutput, 0, 1, 1, ' ', 0)
	fmt.Fprintf(tw, "TIME\tMETRIC\tVALUE\n")
	fmt.Fprintf(tw, "%v\tpings\t5\n", outputTime)
	fmt.Fprintf(tw, "%v\tpings\t5\n", outputTime)
	tw.Flush()
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered", "-n", "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedOutput.String())
}

func (s *DebugMetricsCommandSuite) TestServiceWithNoptionGreaterThanMetricCount(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	newTime := time.Now().Round(time.Second)
	metricA := state.Metric{"pings", "5", newTime}
	metricB := state.Metric{"pings", "10.5", newTime}
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Metrics: []state.Metric{metricA}})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit2, Metrics: []state.Metric{metricA, metricB}})
	outputTime := newTime.Format(time.RFC3339)
	expectedOutput := bytes.Buffer{}
	tw := tabwriter.NewWriter(&expectedOutput, 0, 1, 1, ' ', 0)
	fmt.Fprintf(tw, "TIME\tMETRIC\tVALUE\n")
	fmt.Fprintf(tw, "%v\tpings\t5\n", outputTime)
	fmt.Fprintf(tw, "%v\tpings\t5\n", outputTime)
	fmt.Fprintf(tw, "%v\tpings\t10.5\n", outputTime)
	tw.Flush()
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered", "-n", "42")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedOutput.String())
}

func (s *DebugMetricsCommandSuite) TestNoMetrics(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered", "-n", "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *DebugMetricsCommandSuite) TestUnitJsonOutput(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "local:quantal/metered"})
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
	ctx, err := coretesting.RunCommand(c, metricsdebug.New(), "metered/0", "--json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedOutput)
}
