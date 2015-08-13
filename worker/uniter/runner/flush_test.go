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
	"github.com/juju/juju/worker/uniter/metrics"
	"github.com/juju/juju/worker/uniter/runner"
)

type FlushContextSuite struct {
	HookContextSuite
	stub testing.Stub
}

var _ = gc.Suite(&FlushContextSuite{})

func (s *FlushContextSuite) SetUpTest(c *gc.C) {
	s.HookContextSuite.SetUpTest(c)
	s.stub.ResetCalls()
}

func (s *FlushContextSuite) TestRunHookRelationFlushingError(c *gc.C) {
	ctx := s.context(c)

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
	err = ctx.Flush("some badge", errors.New("blam pow"))
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
	ctx := s.context(c)

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
	err = ctx.Flush("some badge", nil)
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

	ctx := s.context(c)

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
	err = ctx.Flush("some badge", nil)
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
	ctx := s.context(c)
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")

	size := uint64(1)
	ctx.AddUnitStorage(
		map[string]params.StorageConstraints{
			"allecto": params.StorageConstraints{Size: &size},
		})

	// Flush the context with an error.
	msg := "test fail run hook"
	err := ctx.Flush("test fail run hook", errors.New(msg))
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)

	all, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}

func (s *FlushContextSuite) TestRunHookAddUnitStorageOnSuccess(c *gc.C) {
	ctx := s.context(c)
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")

	size := uint64(1)
	ctx.AddUnitStorage(
		map[string]params.StorageConstraints{
			"allecto": params.StorageConstraints{Size: &size},
		})

	// Flush the context with a success.
	err := ctx.Flush("success", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, `.*storage "allecto" not found.*`)

	all, err := s.State.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}

func (s *FlushContextSuite) TestFlushClosesMetricsRecorder(c *gc.C) {
	uuid := utils.MustNewUUID()
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, s.metricsDefinition("key"), NewRealPaths(c))

	runner.PatchMetricsRecorder(ctx, &StubMetricsRecorder{&s.stub})

	err := ctx.AddMetric("key", "value", time.Now())

	// Flush the context with a success.
	err = ctx.Flush("success", nil)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "IsDeclaredMetric", "AddMetric", "Close")
}

func (s *HookContextSuite) context(c *gc.C) *runner.HookContext {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return s.getHookContext(c, uuid.String(), -1, "", noProxies)
}

func (s *FlushContextSuite) TestBuiltinMetric(c *gc.C) {
	uuid := utils.MustNewUUID()
	paths := NewRealPaths(c)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, s.metricsDefinition("juju-units"), paths)
	reader, err := metrics.NewJSONMetricReader(
		paths.GetMetricsSpoolDir(),
	)

	err = ctx.Flush("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)
	batches, err := reader.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 1)
	c.Assert(batches[0].Metrics, gc.HasLen, 1)
	c.Assert(batches[0].Metrics[0].Key, gc.Equals, "juju-units")
	c.Assert(batches[0].Metrics[0].Value, gc.Equals, "1")
}

func (s *FlushContextSuite) TestBuiltinMetricNotGeneratedIfNotDefined(c *gc.C) {
	uuid := utils.MustNewUUID()
	paths := NewRealPaths(c)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, s.metricsDefinition("pings"), paths)
	reader, err := metrics.NewJSONMetricReader(
		paths.GetMetricsSpoolDir(),
	)

	err = ctx.Flush("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)
	batches, err := reader.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 0)
}

func (s *FlushContextSuite) TestRecorderIsClosedAfterBuiltIn(c *gc.C) {
	uuid := utils.MustNewUUID()
	paths := NewRealPaths(c)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, s.metricsDefinition("juju-units"), paths)
	runner.PatchMetricsRecorder(ctx, &StubMetricsRecorder{&s.stub})

	err := ctx.Flush("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "IsDeclaredMetric", "AddMetric", "Close")
}
