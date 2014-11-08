// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/uniter/context"
)

type RunCommandSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&RunCommandSuite{})

func (s *RunCommandSuite) getHookContext(c *gc.C) *context.HookContext {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	return s.HookContextSuite.getHookContext(c, uuid.String(), -1, "", noProxies)
}

func (s *RunCommandSuite) TestRunCommandsEnvStdOutAndErrAndRC(c *gc.C) {
	ctx := s.getHookContext(c)
	paths := NewRealPaths(c)
	runner := context.NewRunner(ctx, paths)

	commands := `
echo $JUJU_CHARM_DIR
echo this is standard err >&2
exit 42
`
	result, err := runner.RunCommands(commands)
	c.Assert(err, gc.IsNil)

	c.Assert(result.Code, gc.Equals, 42)
	c.Assert(string(result.Stdout), gc.Equals, paths.charm+"\n")
	c.Assert(string(result.Stderr), gc.Equals, "this is standard err\n")
	c.Assert(ctx.GetProcess(), gc.Not(gc.IsNil))
}

type RunHookSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&RunHookSuite{})

// LineBufferSize matches the constant used when creating
// the bufio line reader.
const lineBufferSize = 4096

var runHookTests = []struct {
	summary string
	relid   int
	remote  string
	spec    hookSpec
	err     string
}{
	{
		summary: "missing hook is not an error",
		relid:   -1,
	}, {
		summary: "report failure to execute hook",
		relid:   -1,
		spec:    hookSpec{perm: 0600},
		err:     `exec: .*something-happened": permission denied`,
	}, {
		summary: "report error indicated by hook's exit status",
		relid:   -1,
		spec: hookSpec{
			perm: 0700,
			code: 99,
		},
		err: "exit status 99",
	}, {
		summary: "output logging",
		relid:   -1,
		spec: hookSpec{
			perm:   0700,
			stdout: "stdout",
			stderr: "stderr",
		},
	}, {
		summary: "output logging with background process",
		relid:   -1,
		spec: hookSpec{
			perm:       0700,
			stdout:     "stdout",
			background: "not printed",
		},
	}, {
		summary: "long line split",
		relid:   -1,
		spec: hookSpec{
			perm:   0700,
			stdout: strings.Repeat("a", lineBufferSize+10),
		},
	},
}

func (s *RunHookSuite) TestRunHook(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	for i, t := range runHookTests {
		c.Logf("\ntest %d: %s; perm %v", i, t.summary, t.spec.perm)
		ctx := s.getHookContext(c, uuid.String(), t.relid, t.remote, noProxies)
		paths := NewRealPaths(c)
		runner := context.NewRunner(ctx, paths)
		var hookExists bool
		if t.spec.perm != 0 {
			spec := t.spec
			spec.name = "something-happened"
			c.Logf("makeCharm %#v", spec)
			makeCharm(c, spec, paths.charm)
			hookExists = true
		}
		t0 := time.Now()
		err := runner.RunHook("something-happened")
		if t.err == "" && hookExists {
			c.Assert(err, gc.IsNil)
		} else if !hookExists {
			c.Assert(context.IsMissingHookError(err), jc.IsTrue)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
		if t.spec.background != "" && time.Now().Sub(t0) > 5*time.Second {
			c.Errorf("background process holding up hook execution")
		}
	}
}

func (s *RunHookSuite) TestRunHookRelationFlushingError(c *gc.C) {
	// TODO(fwereade): these should be testing a public Flush() method on
	// the context, or something, instead of faking up an unnecessary hook
	// execution.

	// Create a charm with a breaking hook.
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies)
	paths := NewRealPaths(c)
	makeCharm(c, hookSpec{
		name: "something-happened",
		perm: 0700,
		code: 123,
	}, paths.charm)

	// Mess with multiple relation settings.
	relCtx0, ok := ctx.Relation(0)
	c.Assert(ok, jc.IsTrue)
	node0, err := relCtx0.Settings()
	c.Assert(err, gc.IsNil)
	node0.Set("foo", "1")
	relCtx1, ok := ctx.Relation(1)
	c.Assert(ok, jc.IsTrue)
	node1, err := relCtx1.Settings()
	c.Assert(err, gc.IsNil)
	node1.Set("bar", "2")

	// Run the failing hook.
	err = context.NewRunner(ctx, paths).RunHook("something-happened")
	c.Assert(err, gc.ErrorMatches, "exit status 123")

	// Check that the changes have not been written to state.
	settings0, err := s.relunits[0].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings0, gc.DeepEquals, map[string]interface{}{"relation-name": "db0"})
	settings1, err := s.relunits[1].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings1, gc.DeepEquals, map[string]interface{}{"relation-name": "db1"})
}

func (s *RunHookSuite) TestRunHookRelationFlushingSuccess(c *gc.C) {
	// TODO(fwereade): these should be testing a public Flush() method on
	// the context, or something, instead of faking up an unnecessary hook
	// execution.

	// Create a charm with a working hook, and mess with settings again.
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies)
	paths := NewRealPaths(c)
	makeCharm(c, hookSpec{
		name: "something-happened",
		perm: 0700,
	}, paths.charm)

	// Mess with multiple relation settings.
	relCtx0, ok := ctx.Relation(0)
	c.Assert(ok, jc.IsTrue)
	node0, err := relCtx0.Settings()
	c.Assert(err, gc.IsNil)
	node0.Set("baz", "3")
	relCtx1, ok := ctx.Relation(1)
	c.Assert(ok, jc.IsTrue)
	node1, err := relCtx1.Settings()
	c.Assert(err, gc.IsNil)
	node1.Set("qux", "4")

	// Run the hook.
	err = context.NewRunner(ctx, paths).RunHook("something-happened")
	c.Assert(err, gc.IsNil)

	// Check that the changes have been written to state.
	settings0, err := s.relunits[0].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings0, gc.DeepEquals, map[string]interface{}{
		"relation-name": "db0",
		"baz":           "3",
	})
	settings1, err := s.relunits[1].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings1, gc.DeepEquals, map[string]interface{}{
		"relation-name": "db1",
		"qux":           "4",
	})
}

func (s *RunHookSuite) TestRunHookMetricSending(c *gc.C) {
	// TODO(fwereade): these should be testing a public Flush() method on
	// the context, or something, instead of faking up an unnecessary hook
	// execution.
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, s.metricsDefinition("key"))
	paths := NewRealPaths(c)
	makeCharm(c, hookSpec{
		name: "collect-metrics",
		perm: 0700,
	}, paths.charm)

	now := time.Now()
	ctx.AddMetric("key", "50", now)

	// Run the hook.
	err = context.NewRunner(ctx, paths).RunHook("collect-metrics")
	c.Assert(err, gc.IsNil)

	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, gc.IsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	metrics := metricBatches[0].Metrics()
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Key, gc.Equals, "key")
	c.Assert(metrics[0].Value, gc.Equals, "50")
}

func (s *RunHookSuite) TestRunHookMetricSendingDisabled(c *gc.C) {
	// TODO(fwereade): these should be testing a public Flush() method on
	// the context, or something, instead of faking up an unnecessary hook
	// execution.
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, false, s.metricsDefinition("key"))
	paths := NewRealPaths(c)
	makeCharm(c, hookSpec{
		name: "some-hook",
		perm: 0700,
	}, paths.charm)

	now := time.Now()
	err = ctx.AddMetric("key", "50", now)
	c.Assert(err, gc.ErrorMatches, "metrics disabled")

	// Run the hook.
	err = context.NewRunner(ctx, paths).RunHook("some-hook")
	c.Assert(err, gc.IsNil)

	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, gc.IsNil)
	c.Assert(metricBatches, gc.HasLen, 0)
}

func (s *RunHookSuite) TestRunHookMetricSendingUndeclared(c *gc.C) {
	// TODO(fwereade): these should be testing a public Flush() method on
	// the context, or something, instead of faking up an unnecessary hook
	// execution.
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ctx := s.getMeteredHookContext(c, uuid.String(), -1, "", noProxies, true, nil)
	paths := NewRealPaths(c)
	makeCharm(c, hookSpec{
		name: "some-hook",
		perm: 0700,
	}, paths.charm)

	now := time.Now()
	err = ctx.AddMetric("key", "50", now)
	c.Assert(err, gc.ErrorMatches, "metrics disabled")

	// Run the hook.
	err = context.NewRunner(ctx, paths).RunHook("some-hook")
	c.Assert(err, gc.IsNil)

	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, gc.IsNil)
	c.Assert(metricBatches, gc.HasLen, 0)
}

func (s *RunHookSuite) TestRunHookOpensAndClosesPendingPorts(c *gc.C) {
	// TODO(fwereade): these should be testing a public Flush() method on
	// the context, or something, instead of faking up an unnecessary hook
	// execution.
	// Initially, no port ranges are open on the unit or its machine.
	unitRanges, err := s.unit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(unitRanges, gc.HasLen, 0)
	machinePorts, err := s.machine.AllPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(machinePorts, gc.HasLen, 0)

	// Add another unit on the same machine.
	otherUnit, err := s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = otherUnit.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)

	// Open some ports on both units.
	err = s.unit.OpenPorts("tcp", 100, 200)
	c.Assert(err, gc.IsNil)
	err = otherUnit.OpenPorts("udp", 200, 300)
	c.Assert(err, gc.IsNil)

	unitRanges, err = s.unit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(unitRanges, jc.DeepEquals, []network.PortRange{
		{100, 200, "tcp"},
	})

	// Get the context.
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies)
	paths := NewRealPaths(c)
	makeCharm(c, hookSpec{
		name: "some-hook",
		perm: 0700,
	}, paths.charm)

	// Try opening some ports via the context.
	err = ctx.OpenPorts("tcp", 100, 200)
	c.Assert(err, gc.IsNil) // duplicates are ignored
	err = ctx.OpenPorts("udp", 200, 300)
	c.Assert(err, gc.ErrorMatches, `cannot open 200-300/udp \(unit "u/0"\): conflicts with existing 200-300/udp \(unit "u/1"\)`)
	err = ctx.OpenPorts("udp", 100, 200)
	c.Assert(err, gc.ErrorMatches, `cannot open 100-200/udp \(unit "u/0"\): conflicts with existing 200-300/udp \(unit "u/1"\)`)
	err = ctx.OpenPorts("udp", 10, 20)
	c.Assert(err, gc.IsNil)
	err = ctx.OpenPorts("tcp", 50, 100)
	c.Assert(err, gc.ErrorMatches, `cannot open 50-100/tcp \(unit "u/0"\): conflicts with existing 100-200/tcp \(unit "u/0"\)`)
	err = ctx.OpenPorts("tcp", 50, 80)
	c.Assert(err, gc.IsNil)
	err = ctx.OpenPorts("tcp", 40, 90)
	c.Assert(err, gc.ErrorMatches, `cannot open 40-90/tcp \(unit "u/0"\): conflicts with 50-80/tcp requested earlier`)

	// Now try closing some ports as well.
	err = ctx.ClosePorts("udp", 8080, 8088)
	c.Assert(err, gc.IsNil) // not existing -> ignored
	err = ctx.ClosePorts("tcp", 100, 200)
	c.Assert(err, gc.IsNil)
	err = ctx.ClosePorts("tcp", 100, 200)
	c.Assert(err, gc.IsNil) // duplicates are ignored
	err = ctx.ClosePorts("udp", 200, 300)
	c.Assert(err, gc.ErrorMatches, `cannot close 200-300/udp \(opened by "u/1"\) from "u/0"`)
	err = ctx.ClosePorts("tcp", 50, 80)
	c.Assert(err, gc.IsNil) // still pending -> no longer pending

	// Ensure the ports are not actually changed on the unit yet.
	unitRanges, err = s.unit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(unitRanges, jc.DeepEquals, []network.PortRange{
		{100, 200, "tcp"},
	})

	// Simulate a hook ran and was committed successfully.
	err = context.NewRunner(ctx, paths).RunHook("some-hook")
	c.Assert(err, gc.IsNil)

	// Verify the unit ranges are now open.
	expectUnitRanges := []network.PortRange{
		{FromPort: 10, ToPort: 20, Protocol: "udp"},
	}
	unitRanges, err = s.unit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(unitRanges, jc.DeepEquals, expectUnitRanges)
}
