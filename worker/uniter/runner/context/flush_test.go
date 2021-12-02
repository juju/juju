// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	runnertesting "github.com/juju/juju/worker/uniter/runner/testing"
)

const allEndpoints = ""

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
	relCtx0, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)
	node0, err := relCtx0.Settings()
	c.Assert(err, jc.ErrorIsNil)
	node0.Set("foo", "1")
	relCtx1, err := ctx.Relation(1)
	c.Assert(err, jc.ErrorIsNil)
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
	relCtx0, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)
	node0, err := relCtx0.Settings()
	c.Assert(err, jc.ErrorIsNil)
	node0.Set("baz", "3")
	relCtx1, err := ctx.Relation(1)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *FlushContextSuite) TestRebootAfterHook(c *gc.C) {
	ctx := s.context(c)

	// Set reboot priority
	err := ctx.RequestReboot(jujuc.RebootAfterHook)
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context with an error and check that reboot is not triggered.
	expErr := errors.New("hook execution failed")
	err = ctx.Flush("some badge", expErr)
	c.Assert(err, gc.Equals, expErr)

	reboot, err := s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reboot, jc.IsFalse, gc.Commentf("expected reboot request not to be triggered for unit's machine"))

	// Flush the context without an error and check that reboot is triggered.
	err = ctx.Flush("some badge", nil)
	c.Assert(err, gc.Equals, context.ErrReboot)

	reboot, err = s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reboot, jc.IsTrue, gc.Commentf("expected reboot request to be triggered for unit's machine"))
}

func (s *FlushContextSuite) TestRebootWhenHookFails(c *gc.C) {
	ctx := s.context(c)

	var stub testing.Stub
	ctx.SetProcess(&mockProcess{func() error {
		priority := ctx.GetRebootPriority()
		c.Assert(priority, gc.Equals, jujuc.RebootAfterHook)
		return stub.NextErr()
	}})
	stub.SetErrors(errors.New("process is already dead"))

	// Set reboot priority
	err := ctx.RequestReboot(jujuc.RebootAfterHook)
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context with an error and check that reboot is not triggered.
	expErr := errors.New("hook execution failed")
	err = ctx.Flush("some badge", expErr)
	c.Assert(err, gc.ErrorMatches, "hook execution failed")

	reboot, err := s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reboot, jc.IsFalse)
}

func (s *FlushContextSuite) TestRebootNowWhenHookFails(c *gc.C) {
	ctx := s.context(c)

	var stub testing.Stub
	ctx.SetProcess(&mockProcess{func() error {
		priority := ctx.GetRebootPriority()
		c.Assert(priority, gc.Equals, jujuc.RebootNow)
		return stub.NextErr()
	}})
	stub.SetErrors(errors.New("process is already dead"))

	// Set reboot priority
	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context with an error and check that reboot is triggered regardless.
	expErr := errors.New("hook execution failed")
	err = ctx.Flush("some badge", expErr)
	c.Assert(err, gc.Equals, context.ErrRequeueAndReboot)

	reboot, err := s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reboot, jc.IsTrue, gc.Commentf("expected reboot request to be triggered for unit's machine"))
}

func (s *FlushContextSuite) TestRebootNow(c *gc.C) {
	ctx := s.context(c)

	var stub testing.Stub
	ctx.SetProcess(&mockProcess{func() error {
		priority := ctx.GetRebootPriority()
		c.Assert(priority, gc.Equals, jujuc.RebootNow)
		return stub.NextErr()
	}})
	stub.SetErrors(errors.New("process is already dead"))

	// Set reboot priority
	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context without an error and check that reboot is triggered.
	err = ctx.Flush("some badge", nil)
	c.Assert(err, gc.Equals, context.ErrRequeueAndReboot)

	reboot, err := s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reboot, jc.IsTrue, gc.Commentf("expected reboot request to be triggered for unit's machine"))
}

func (s *FlushContextSuite) TestRunHookOpensAndClosesPendingPorts(c *gc.C) {
	// Initially, no port ranges are open on the unit or its machine.
	machPortRanges, err := s.machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machPortRanges.UniquePortRanges(), gc.HasLen, 0)

	// Add another unit on the same machine.
	otherUnit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = otherUnit.AssignToMachine(s.machine)
	c.Assert(err, jc.ErrorIsNil)

	// Open some ports on both units.
	mustOpenPortRanges(c, s.State, s.unit, allEndpoints, []network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
	})
	mustOpenPortRanges(c, s.State, otherUnit, allEndpoints, []network.PortRange{
		network.MustParsePortRange("200-300/udp"),
	})

	ctx := s.context(c)

	// Try opening some ports via the context.
	err = ctx.OpenPortRange("", network.MustParsePortRange("100-200/tcp"))
	c.Assert(err, jc.ErrorIsNil) // duplicates are ignored
	err = ctx.OpenPortRange("", network.MustParsePortRange("200-300/udp"))
	c.Assert(err, gc.ErrorMatches, `cannot open 200-300/udp \(unit "u/0"\): port range conflicts with 200-300/udp \(unit "u/1"\)`)
	err = ctx.OpenPortRange("", network.MustParsePortRange("100-200/udp"))
	c.Assert(err, gc.ErrorMatches, `cannot open 100-200/udp \(unit "u/0"\): port range conflicts with 200-300/udp \(unit "u/1"\)`)
	err = ctx.OpenPortRange("", network.MustParsePortRange("10-20/udp"))
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.OpenPortRange("", network.MustParsePortRange("50-100/tcp"))
	c.Assert(err, gc.ErrorMatches, `cannot open 50-100/tcp \(unit "u/0"\): port range conflicts with 100-200/tcp \(unit "u/0"\)`)
	err = ctx.OpenPortRange("", network.MustParsePortRange("50-80/tcp"))
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.OpenPortRange("", network.MustParsePortRange("40-90/tcp"))
	c.Assert(err, gc.ErrorMatches, `cannot open 40-90/tcp \(unit "u/0"\): port range conflicts with 50-80/tcp \(unit "u/0"\) requested earlier`)

	// Now try closing some ports as well.
	err = ctx.ClosePortRange("", network.MustParsePortRange("8080-8088/udp"))
	c.Assert(err, jc.ErrorIsNil) // not existing -> ignored
	err = ctx.ClosePortRange("", network.MustParsePortRange("100-200/tcp"))
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.ClosePortRange("", network.MustParsePortRange("100-200/tcp"))
	c.Assert(err, jc.ErrorIsNil) // duplicates are ignored
	err = ctx.ClosePortRange("", network.MustParsePortRange("200-300/udp"))
	c.Assert(err, gc.ErrorMatches, `.*port range conflicts with 200-300/udp \(unit "u/1"\)`)
	err = ctx.ClosePortRange("", network.MustParsePortRange("50-80/tcp"))
	c.Assert(err, jc.ErrorIsNil) // still pending -> no longer pending

	// Ensure the ports are not actually changed on the unit yet.
	unitPortRanges, err := s.unit.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitPortRanges.UniquePortRanges(), jc.DeepEquals, []network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
	})

	// Flush the context with a success.
	err = ctx.Flush("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Verify the unit ranges are now open.
	unitPortRanges, err = s.unit.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitPortRanges.UniquePortRanges(), jc.DeepEquals, []network.PortRange{
		network.MustParsePortRange("10-20/udp"),
	})
}

func (s *FlushContextSuite) TestRunHookAddStorageOnFailure(c *gc.C) {
	ctx := s.context(c)
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")

	size := uint64(1)
	ctx.AddUnitStorage(
		map[string]params.StorageConstraints{
			"allecto": {Size: &size},
		})

	// Flush the context with an error.
	msg := "test fail run hook"
	err := ctx.Flush("test fail run hook", errors.New(msg))
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	all, err := sb.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}

func (s *FlushContextSuite) TestRunHookAddUnitStorageOnSuccess(c *gc.C) {
	ctx := s.context(c)
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")

	size := uint64(1)
	ctx.AddUnitStorage(
		map[string]params.StorageConstraints{
			"allecto": {Size: &size},
		})

	// Flush the context with a success.
	err := ctx.Flush("success", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, `.*storage "allecto" not found.*`)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	all, err := sb.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}

func (s *HookContextSuite) context(c *gc.C) *context.HookContext {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return s.getHookContext(c, uuid.String(), -1, "", names.StorageTag{})
}

func (s *FlushContextSuite) TestBuiltinMetricNotGeneratedIfNotDefined(c *gc.C) {
	uuid := utils.MustNewUUID()
	paths := runnertesting.NewRealPaths(c)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", true, s.metricsDefinition("pings"), paths)
	reader, err := spool.NewJSONMetricReader(
		paths.GetMetricsSpoolDir(),
	)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.Flush("some badge", nil)
	c.Assert(err, jc.ErrorIsNil)
	batches, err := reader.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 0)
}

func mustOpenPortRanges(c *gc.C, st *state.State, u *state.Unit, endpointName string, portRanges []network.PortRange) {
	unitPortRanges, err := u.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	for _, pr := range portRanges {
		unitPortRanges.Open(endpointName, pr)
	}

	c.Assert(st.ApplyOperation(unitPortRanges.Changes()), jc.ErrorIsNil)
}
