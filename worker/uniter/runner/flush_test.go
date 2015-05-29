// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type FlushContextSuite struct {
	HookContextSuite
	testing.Stub
}

var _ = gc.Suite(&FlushContextSuite{})

// StubMetricsReader is a stub implementation of the metrics reader.
type StubMetricsReader struct {
	*testing.Stub
	Batches []runner.MetricsBatch
}

// Open implements the MetricsReader interface.
func (mr *StubMetricsReader) Open() ([]runner.MetricsBatch, error) {
	mr.MethodCall(mr, "Open")
	return mr.Batches, mr.NextErr()
}

// Remove implements the MetricsReader interface.
func (mr *StubMetricsReader) Remove(uuid string) error {
	mr.MethodCall(mr, "Remove", uuid)
	return mr.NextErr()
}

// Close implements the MetricsReader interface.
func (mr *StubMetricsReader) Close() error {
	mr.MethodCall(mr, "Close")
	return mr.NextErr()
}

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

func (s *FlushContextSuite) TestRunHookMetricSendingSuccess(c *gc.C) {
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

func (s *FlushContextSuite) TestRunHookMetricSendingGetDuplicate(c *gc.C) {
	uuid := utils.MustNewUUID()
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, s.metricsDefinition("pings"))

	// Send batches once.
	batches := []runner.MetricsBatch{
		{
			CharmURL: s.meteredCharm.URL().String(),
			UUID:     utils.MustNewUUID().String(),
			Created:  time.Now(),
			Metrics:  []jujuc.Metric{{Key: "pings", Value: "1", Time: time.Now()}},
		}, {
			CharmURL: s.meteredCharm.URL().String(),
			UUID:     utils.MustNewUUID().String(),
			Created:  time.Now(),
			Metrics:  []jujuc.Metric{{Key: "pings", Value: "1", Time: time.Now()}},
		},
	}

	reader := &StubMetricsReader{
		Stub:    &s.Stub,
		Batches: batches,
	}

	runner.PatchMetricsReader(ctx, reader)

	// Flush the context with a success.
	err := ctx.FlushContext("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Check stub calls.
	s.Stub.CheckCallNames(c, "Open", "Remove", "Remove", "Close")
	s.Stub.ResetCalls()
	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricBatches, gc.HasLen, 2)

	// Create a new context with a duplicate metrics batch.
	uuid = utils.MustNewUUID()
	ctx = s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, s.metricsDefinition("pings"))
	runner.PatchMetricsReader(ctx, reader)

	newBatches := []runner.MetricsBatch{
		batches[0],
		{
			CharmURL: s.meteredCharm.URL().String(),
			UUID:     utils.MustNewUUID().String(),
			Created:  time.Now(),
			Metrics:  []jujuc.Metric{{Key: "pings", Value: "1", Time: time.Now()}},
		},
	}
	reader.Batches = newBatches

	// Flush the context with a success.
	err = ctx.FlushContext("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Check stub calls.
	s.Stub.CheckCallNames(c, "Open", "Remove", "Remove", "Close")

	metricBatches, err = s.State.MetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	// Only one additional metric has been recorded.
	c.Assert(metricBatches, gc.HasLen, 3)

}

func (s *FlushContextSuite) TestRunHookNoMetricSendingOnFailure(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, s.metricsDefinition("key"))

	now := time.Now()
	ctx.AddMetric("key", "50", now)

	// Flush the context with an error.
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

func (s *FlushContextSuite) TestRunHookAddStorageOnFailure(c *gc.C) {
	// Get the context.
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies)
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")

	size := uint64(1)
	ctx.AddUnitStorage(
		map[string]params.StorageConstraints{
			"allecto": params.StorageConstraints{Size: &size},
		})

	// Flush the context with an error.
	msg := "test fail run hook"
	err = ctx.FlushContext("test fail run hook", errors.New(msg))
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)

	all, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}

func (s *FlushContextSuite) TestRunHookAddUnitStorageOnSuccess(c *gc.C) {
	// Get the context.
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies)
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")

	size := uint64(1)
	ctx.AddUnitStorage(
		map[string]params.StorageConstraints{
			"allecto": params.StorageConstraints{Size: &size},
		})

	// Flush the context with a success.
	err = ctx.FlushContext("success", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, `.*charm storage "allecto" not found.*`)

	all, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}
