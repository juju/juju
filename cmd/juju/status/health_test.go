// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
//
// These tests build on a lot of the framework from status_test.go

package status

import (
	"bytes"

	gc "gopkg.in/check.v1"

	"github.com/juju/cmd"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
)

type expectHealth struct {
	what       string
	stdout     string
	returncode int
}

func (e expectHealth) step(c *gc.C, ctx *context) {
	c.Logf("\nexpect: %s\n", e.what)

	// Now execute Juju Health
	code, stdout, stderr := runHealth(c)
	c.Assert(code, gc.Equals, e.returncode)
	if !c.Check(stderr, gc.HasLen, 0) {
		c.Fatalf("health failed: %s", string(stderr))
	}
	c.Assert(string(stdout), gc.Equals, e.stdout)
}

func runHealth(c *gc.C) (code int, stdout, stderr []byte) {
	ctx := coretesting.Context(c)
	code = cmd.Main(NewHealthCommand(), ctx, []string{""})
	stdout = ctx.Stdout.(*bytes.Buffer).Bytes()
	stderr = ctx.Stderr.(*bytes.Buffer).Bytes()
	return
}

var healthTests = []testCase{
	// Status tests
	test( // 0
		"bootstrap and starting a single instance",

		// machine tests
		addMachine{machineId: "0", job: state.JobManageModel},
		startAliveMachine{"0"},
		setAddresses{"0", []network.Address{
			network.NewAddress("10.0.0.1"),
			network.NewScopedAddress("admin-0.dns", network.ScopePublic),
		}},
		expectHealth{
			"simulate juju bootstrap by adding machine/0 to the state",
			"Juju Health Warning\n" +
				"Machine: 0\t Status: pending\n",
			1},
		setMachineStatus{"0", status.StatusError, "broken"},
		expectHealth{
			"simulate machine error",
			"Juju Health Critical\n" +
				"Machine: 0\t Status: error\n",
			2},
		setMachineStatus{"0", status.StatusStarted, ""},
		expectHealth{
			"simulate the MA started and set the machine status",
			"Juju Health Okay\n",
			0},

		// container tests
		addContainer{"0", "0/lxc/0", state.JobHostUnits},
		expectHealth{
			"container pending",
			"Juju Health Warning\n" +
				"Machine: 0\t Container: 0/lxc/0\t Status: pending\n",
			1},
		setMachineStatus{"0/lxc/0", status.StatusError, "broken"},
		expectHealth{
			"simulate container error",
			"Juju Health Critical\n" +
				"Machine: 0\t Container: 0/lxc/0\t Status: error\n",
			2},
		setMachineStatus{"0/lxc/0", status.StatusStarted, ""},
		expectHealth{
			"simulate container fixed",
			"Juju Health Okay\n",
			0},
		addContainer{"0/lxc/0", "0/lxc/0/lxc/0", state.JobHostUnits},
		expectHealth{
			"nested container pending",
			"Juju Health Warning\n" +
				"Machine: 0\t Container: 0/lxc/0\t Container: 0/lxc/0/lxc/0\t Status: pending\n",
			1},
		setMachineStatus{"0/lxc/0/lxc/0", status.StatusError, "broken"},
		expectHealth{
			"nested container error",
			"Juju Health Critical\n" +
				"Machine: 0\t Container: 0/lxc/0\t Container: 0/lxc/0/lxc/0\t Status: error\n",
			2},
		setMachineStatus{"0/lxc/0/lxc/0", status.StatusStarted, ""},
		expectHealth{
			"nested container fixed",
			"Juju Health Okay\n",
			0},

		// unit/service tests
		addCharm{"wordpress"},
		addService{name: "wordpress", charm: "wordpress"},
		setServiceExposed{"wordpress", true},
		addMachine{machineId: "1", job: state.JobHostUnits},
		startAliveMachine{"1"},
		expectHealth{
			"Machine 1 coming up",
			"Juju Health Warning\n" +
				"Machine: 1\t Status: pending\n" +
				"Service: wordpress\t Status: unknown, Error: <nil>\n",
			1},
		setMachineStatus{"1", status.StatusStarted, ""},
		expectHealth{
			"Machine ready, service not",
			"Juju Health Warning\n" +
				"Service: wordpress\t Status: unknown, Error: <nil>\n",
			1},
		addAliveUnit{"wordpress", "1"},
		setAgentStatus{"wordpress/0", status.StatusIdle, "", nil},
		setUnitStatus{"wordpress/0", status.StatusActive, "", nil},
		expectHealth{
			"Agent and Unit Ready",
			"Juju Health Okay\n",
			0},
		setAgentStatus{"wordpress/0", status.StatusError, "Press more", nil},
		expectHealth{
			"Simulate failed agent error",
			"Juju Health Critical\n" +
				"Service: wordpress\t Status: error, Error: <nil>\n" +
				"Service: wordpress\t Unit: wordpress/0\t WorkloadStatus: error\n",
			2},
		setAgentStatus{"wordpress/0", status.StatusIdle, "running", nil},
		expectHealth{
			"Fixed agent status",
			"Juju Health Okay\n",
			0},
		setUnitStatus{"wordpress/0", status.StatusBlocked, "", nil},
		expectHealth{
			"Simulate Blocked unit status",
			"Juju Health Critical\n" +
				"Service: wordpress\t Status: blocked, Error: <nil>\n" +
				"Service: wordpress\t Unit: wordpress/0\t WorkloadStatus: blocked\n",
			2},
		setUnitStatus{"wordpress/0", status.StatusActive, "", nil},
		expectHealth{
			"Fixed agent status",
			"Juju Health Okay\n",
			0},

		// subordinate tests
		addCharm{"logging"},
		addService{name: "logging", charm: "logging"},
		setServiceExposed{"logging", true},
		relateServices{"wordpress", "logging"},
		addSubordinate{"wordpress/0", "logging"},
		setUnitsAlive{"logging"},
		setAgentStatus{"logging/0", status.StatusIdle, "", nil},
		setUnitStatus{"logging/0", status.StatusTerminated, "", nil},
		expectHealth{
			"Simulate Terminated unit status",
			"Juju Health Warning\n" +
				"Service: wordpress\t Unit: wordpress/0\t Subordinate: logging/0\t WorkloadStatus: terminated\n",
			1},
		setUnitStatus{"logging/0", status.StatusActive, "", nil},
		expectHealth{
			"Fixed unit status",
			"Juju Health Okay\n",
			0},
	),
}

func (s *StatusSuite) TestHealth(c *gc.C) {
	c.Log("TestHealth:")
	for i, t := range healthTests {
		c.Logf("test %d: %s", i, t.summary)
		func(t testCase) {
			// Prepare context and run all steps to setup.
			ctx := s.newContext(c)
			defer s.resetContext(c, ctx)
			ctx.run(c, t.steps)
		}(t)
	}
}
