// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/uniter/context"
)

type RealPaths struct {
	tools  string
	charm  string
	socket string
}

func NewRealPaths(c *gc.C) RealPaths {
	return RealPaths{
		tools:  c.MkDir(),
		charm:  c.MkDir(),
		socket: filepath.Join(c.MkDir(), "jujuc.socket"),
	}
}

func (p RealPaths) GetToolsDir() string {
	return p.tools
}

func (p RealPaths) GetCharmDir() string {
	return p.charm
}

func (p RealPaths) GetJujucSocket() string {
	return p.socket
}

type RunCommandSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&RunCommandSuite{})

func (s *RunCommandSuite) getHookContext(c *gc.C, addMetrics bool) *context.HookContext {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	return s.HookContextSuite.getHookContext(c, uuid.String(), -1, "", noProxies, addMetrics)
}

func (s *RunCommandSuite) TestRunCommandsEnvStdOutAndErrAndRC(c *gc.C) {
	ctx := s.getHookContext(c, false)
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
}

type RunHookSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&RunHookSuite{})

type hookSpec struct {
	// name is the name of the hook.
	name string
	// perm is the file permissions of the hook.
	perm os.FileMode
	// code is the exit status of the hook.
	code int
	// stdout holds a string to print to stdout
	stdout string
	// stderr holds a string to print to stderr
	stderr string
	// background holds a string to print in the background after 0.2s.
	background string
}

// makeCharm constructs a fake charm dir containing a single named hook
// with permissions perm and exit code code.  If output is non-empty,
// the charm will write it to stdout and stderr, with each one prefixed
// by name of the stream.
func makeCharm(c *gc.C, spec hookSpec, charmDir string) {
	hooksDir := filepath.Join(charmDir, "hooks")
	err := os.Mkdir(hooksDir, 0755)
	c.Assert(err, gc.IsNil)
	c.Logf("openfile perm %v", spec.perm)
	hook, err := os.OpenFile(filepath.Join(hooksDir, spec.name), os.O_CREATE|os.O_WRONLY, spec.perm)
	c.Assert(err, gc.IsNil)
	defer hook.Close()

	printf := func(f string, a ...interface{}) {
		_, err := fmt.Fprintf(hook, f+"\n", a...)
		c.Assert(err, gc.IsNil)
	}
	printf("#!/bin/bash")
	if spec.stdout != "" {
		printf("echo %s", spec.stdout)
	}
	if spec.stderr != "" {
		printf("echo %s >&2", spec.stderr)
	}
	if spec.background != "" {
		// Print something fairly quickly, then sleep for
		// quite a long time - if the hook execution is
		// blocking because of the background process,
		// the hook execution will take much longer than
		// expected.
		printf("(sleep 0.2; echo %s; sleep 10) &", spec.background)
	}
	printf("exit %d", spec.code)
}

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
		ctx := s.getHookContext(c, uuid.String(), t.relid, t.remote, noProxies, false)
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

func (s *RunHookSuite) TestRunHookRelationFlushing(c *gc.C) {
	// TODO(fwereade): these should be testing a public Flush() method on
	// the context, or something, instead of faking up an unnecessary hook
	// execution.

	// Create a charm with a breaking hook.
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies, false)
	paths := NewRealPaths(c)
	makeCharm(c, hookSpec{
		name: "something-happened",
		perm: 0700,
		code: 123,
	}, paths.charm)

	// Mess with multiple relation settings.
	node0, err := s.relctxs[0].Settings()
	node0.Set("foo", "1")
	node1, err := s.relctxs[1].Settings()
	node1.Set("bar", "2")

	// Run the failing hook.
	err = context.NewRunner(ctx, paths).RunHook("something-happened")
	c.Assert(err, gc.ErrorMatches, "exit status 123")

	// Check that the changes to the local settings nodes have been discarded.
	node0, err = s.relctxs[0].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node0.Map(), gc.DeepEquals, params.RelationSettings{"relation-name": "db0"})
	node1, err = s.relctxs[1].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node1.Map(), gc.DeepEquals, params.RelationSettings{"relation-name": "db1"})

	// Check that the changes have not been written to state.
	settings0, err := s.relunits[0].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings0, gc.DeepEquals, map[string]interface{}{"relation-name": "db0"})
	settings1, err := s.relunits[1].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings1, gc.DeepEquals, map[string]interface{}{"relation-name": "db1"})

	// Create a charm with a working hook, and mess with settings again.
	paths = NewRealPaths(c)
	makeCharm(c, hookSpec{
		name: "something-happened",
		perm: 0700,
	}, paths.charm)
	node0.Set("baz", "3")
	node1.Set("qux", "4")

	// Run the hook.
	err = context.NewRunner(ctx, paths).RunHook("something-happened")
	c.Assert(err, gc.IsNil)

	// Check that the changes to the local settings nodes are still there.
	node0, err = s.relctxs[0].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node0.Map(), gc.DeepEquals, params.RelationSettings{
		"relation-name": "db0",
		"baz":           "3",
	})
	node1, err = s.relctxs[1].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node1.Map(), gc.DeepEquals, params.RelationSettings{
		"relation-name": "db1",
		"qux":           "4",
	})

	// Check that the changes have been written to state.
	settings0, err = s.relunits[0].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings0, gc.DeepEquals, map[string]interface{}{
		"relation-name": "db0",
		"baz":           "3",
	})
	settings1, err = s.relunits[1].ReadSettings("u/0")
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
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies, true)
	paths := NewRealPaths(c)
	makeCharm(c, hookSpec{
		name: "collect-metrics",
		perm: 0700,
	}, paths.charm)

	now := time.Now()
	ctx.AddMetrics("key", "50", now)

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
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies, false)
	paths := NewRealPaths(c)
	makeCharm(c, hookSpec{
		name: "some-hook",
		perm: 0700,
	}, paths.charm)

	now := time.Now()
	err = ctx.AddMetrics("key", "50", now)
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
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies, false)
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
