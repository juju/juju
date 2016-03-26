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
	"github.com/juju/version"
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

		addMachine{machineId: "0", job: state.JobManageModel},
		expectHealth{
			"simulate juju bootstrap by adding machine/0 to the state",
			"Juju Health Warning\nMachine: 0\t Status: pending\n",
			1},
		startAliveMachine{"0"},
		setAddresses{"0", []network.Address{
			network.NewAddress("10.0.0.1"),
			network.NewScopedAddress("admin-0.dns", network.ScopePublic),
		}},
		expectHealth{
			"simulate the PA starting an instance in response to the state change",
			"Juju Health Warning\nMachine: 0\t Status: pending\n",
			1},
		setMachineStatus{"0", status.StatusStarted, ""},
		expectHealth{
			"simulate the MA started and set the machine status",
			"Juju Health Okay\n",
			0},
		setTools{"0", version.MustParseBinary("1.2.3-trusty-ppc")},
		expectHealth{
			"simulate the MA setting the version",
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
