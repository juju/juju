// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

type FlushContextSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&FlushContextSuite{})

func (s *FlushContextSuite) TestRunHookRelationFlushingError(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies)

	// Mess with multiple relation settings.
	relCtx0, ok := ctx.Relation(0)
	c.Assert(ok, jc.IsTrue)
	node0, err := relCtx0.Settings()
	c.Assert(err, jc.ErrorIsNil)
	node0.Set("foo", "1")
	relCtx1, ok := ctx.Relation(1)
	c.Assert(ok, jc.IsTrue)
	node1, err := relCtx1.Settings()
	c.Assert(err, jc.ErrorIsNil)
	node1.Set("bar", "2")

	// Flush the context with a failure.
	err = ctx.FlushContext("some badge", errors.New("blam pow"))
	c.Assert(err, gc.ErrorMatches, "blam pow")

	// Check that the changes have not been written to state.
	settings0, err := s.relunits[0].ReadSettings("u/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings0, gc.DeepEquals, map[string]interface{}{"relation-name": "db0"})
	settings1, err := s.relunits[1].ReadSettings("u/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings1, gc.DeepEquals, map[string]interface{}{"relation-name": "db1"})
}

func (s *FlushContextSuite) TestRunHookRelationFlushingSuccess(c *gc.C) {
	// Create a charm with a working hook, and mess with settings again.
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies)

	// Mess with multiple relation settings.
	relCtx0, ok := ctx.Relation(0)
	c.Assert(ok, jc.IsTrue)
	node0, err := relCtx0.Settings()
	c.Assert(err, jc.ErrorIsNil)
	node0.Set("baz", "3")
	relCtx1, ok := ctx.Relation(1)
	c.Assert(ok, jc.IsTrue)
	node1, err := relCtx1.Settings()
	c.Assert(err, jc.ErrorIsNil)
	node1.Set("qux", "4")

	// Flush the context with a success.
	err = ctx.FlushContext("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the changes have been written to state.
	settings0, err := s.relunits[0].ReadSettings("u/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings0, gc.DeepEquals, map[string]interface{}{
		"relation-name": "db0",
		"baz":           "3",
	})
	settings1, err := s.relunits[1].ReadSettings("u/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings1, gc.DeepEquals, map[string]interface{}{
		"relation-name": "db1",
		"qux":           "4",
	})
}

func (s *FlushContextSuite) TestRunHookMetricSending(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, s.metricsDefinition("pings"))

	now := time.Now()
	err = ctx.AddMetric("pings", "50", now)
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context with a success.
	err = ctx.FlushContext("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)

	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	metrics := metricBatches[0].Metrics()
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Key, gc.Equals, "pings")
	c.Assert(metrics[0].Value, gc.Equals, "50")
}

func (s *FlushContextSuite) TestRunHookNoMetricSendingOnFailure(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, s.metricsDefinition("key"))

	now := time.Now()
	ctx.AddMetric("key", "50", now)

	// Flush the context with a success.
	err = ctx.FlushContext("some badge", errors.New("boom squelch"))
	c.Assert(err, gc.ErrorMatches, "boom squelch")

	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 0)
}

func (s *FlushContextSuite) TestRunHookMetricSendingDisabled(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, false, s.metricsDefinition("key"))

	now := time.Now()
	err = ctx.AddMetric("key", "50", now)
	c.Assert(err, gc.ErrorMatches, "metrics disabled")

	// Flush the context with a success.
	err = ctx.FlushContext("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)

	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 0)
}

func (s *FlushContextSuite) TestRunHookMetricSendingUndeclared(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, nil)

	now := time.Now()
	err = ctx.AddMetric("key", "50", now)
	c.Assert(err, gc.ErrorMatches, "metrics disabled")

	// Flush the context with a success.
	err = ctx.FlushContext("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)

	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 0)
}

func (s *FlushContextSuite) TestRunHookOpensAndClosesPendingPorts(c *gc.C) {
	// Initially, no port ranges are open on the unit or its machine.
	unitRanges, err := s.unit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitRanges, gc.HasLen, 0)
	machinePorts, err := s.machine.AllPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machinePorts, gc.HasLen, 0)

	// Add another unit on the same machine.
	otherUnit, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = otherUnit.AssignToMachine(s.machine)
	c.Assert(err, jc.ErrorIsNil)

	// Open some ports on both units.
	err = s.unit.OpenPorts("tcp", 100, 200)
	c.Assert(err, jc.ErrorIsNil)
	err = otherUnit.OpenPorts("udp", 200, 300)
	c.Assert(err, jc.ErrorIsNil)

	unitRanges, err = s.unit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitRanges, jc.DeepEquals, []network.PortRange{
		{100, 200, "tcp"},
	})

	// Get the context.
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies)

	// Try opening some ports via the context.
	err = ctx.OpenPorts("tcp", 100, 200)
	c.Assert(err, jc.ErrorIsNil) // duplicates are ignored
	err = ctx.OpenPorts("udp", 200, 300)
	c.Assert(err, gc.ErrorMatches, `cannot open 200-300/udp \(unit "u/0"\): conflicts with existing 200-300/udp \(unit "u/1"\)`)
	err = ctx.OpenPorts("udp", 100, 200)
	c.Assert(err, gc.ErrorMatches, `cannot open 100-200/udp \(unit "u/0"\): conflicts with existing 200-300/udp \(unit "u/1"\)`)
	err = ctx.OpenPorts("udp", 10, 20)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.OpenPorts("tcp", 50, 100)
	c.Assert(err, gc.ErrorMatches, `cannot open 50-100/tcp \(unit "u/0"\): conflicts with existing 100-200/tcp \(unit "u/0"\)`)
	err = ctx.OpenPorts("tcp", 50, 80)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.OpenPorts("tcp", 40, 90)
	c.Assert(err, gc.ErrorMatches, `cannot open 40-90/tcp \(unit "u/0"\): conflicts with 50-80/tcp requested earlier`)

	// Now try closing some ports as well.
	err = ctx.ClosePorts("udp", 8080, 8088)
	c.Assert(err, jc.ErrorIsNil) // not existing -> ignored
	err = ctx.ClosePorts("tcp", 100, 200)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.ClosePorts("tcp", 100, 200)
	c.Assert(err, jc.ErrorIsNil) // duplicates are ignored
	err = ctx.ClosePorts("udp", 200, 300)
	c.Assert(err, gc.ErrorMatches, `cannot close 200-300/udp \(opened by "u/1"\) from "u/0"`)
	err = ctx.ClosePorts("tcp", 50, 80)
	c.Assert(err, jc.ErrorIsNil) // still pending -> no longer pending

	// Ensure the ports are not actually changed on the unit yet.
	unitRanges, err = s.unit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitRanges, jc.DeepEquals, []network.PortRange{
		{100, 200, "tcp"},
	})

	// Flush the context with a success.
	err = ctx.FlushContext("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Verify the unit ranges are now open.
	expectUnitRanges := []network.PortRange{
		{FromPort: 10, ToPort: 20, Protocol: "udp"},
	}
	unitRanges, err = s.unit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitRanges, jc.DeepEquals, expectUnitRanges)
}
