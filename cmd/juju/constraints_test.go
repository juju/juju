// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
)

type ConstraintsCommandsSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&ConstraintsCommandsSuite{})

func runCmdLine(c *gc.C, com cmd.Command, args ...string) (code int, stdout, stderr string) {
	ctx := coretesting.Context(c)
	code = cmd.Main(com, ctx, args)
	stdout = ctx.Stdout.(*bytes.Buffer).String()
	stderr = ctx.Stderr.(*bytes.Buffer).String()
	c.Logf("args:   %#v\ncode:   %d\nstdout: %q\nstderr: %q", args, code, stdout, stderr)
	return
}

func uint64p(val uint64) *uint64 {
	return &val
}

func assertSet(c *gc.C, args ...string) {
	rcode, rstdout, rstderr := runCmdLine(c, envcmd.Wrap(&SetConstraintsCommand{}), args...)
	c.Assert(rcode, gc.Equals, 0)
	c.Assert(rstdout, gc.Equals, "")
	c.Assert(rstderr, gc.Equals, "")
}

func (s *ConstraintsCommandsSuite) TestSetEnviron(c *gc.C) {
	// Set constraints.
	assertSet(c, "mem=4G", "cpu-power=250")
	cons, err := s.State.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, constraints.Value{
		CpuPower: uint64p(250),
		Mem:      uint64p(4096),
	})

	// Clear constraints.
	assertSet(c)
	cons, err = s.State.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)
}

func (s *ConstraintsCommandsSuite) TestSetService(c *gc.C) {
	svc := s.AddTestingService(c, "svc", s.AddTestingCharm(c, "dummy"))

	// Set constraints.
	assertSet(c, "-s", "svc", "mem=4G", "cpu-power=250")
	cons, err := svc.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, constraints.Value{
		CpuPower: uint64p(250),
		Mem:      uint64p(4096),
	})

	// Clear constraints.
	assertSet(c, "-s", "svc")
	cons, err = svc.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)
}

func assertSetError(c *gc.C, code int, stderr string, args ...string) {
	rcode, rstdout, rstderr := runCmdLine(c, envcmd.Wrap(&SetConstraintsCommand{}), args...)
	c.Assert(rcode, gc.Equals, code)
	c.Assert(rstdout, gc.Equals, "")
	c.Assert(rstderr, gc.Matches, "error: "+stderr+"\n")
}

func (s *ConstraintsCommandsSuite) TestSetErrors(c *gc.C) {
	assertSetError(c, 2, `invalid service name "badname-0"`, "-s", "badname-0")
	assertSetError(c, 2, `malformed constraint "="`, "=")
	assertSetError(c, 2, `malformed constraint "="`, "-s", "s", "=")
	assertSetError(c, 1, `service "missing" not found`, "-s", "missing")
}

func assertGet(c *gc.C, stdout string, args ...string) {
	rcode, rstdout, rstderr := runCmdLine(c, &GetConstraintsCommand{}, args...)
	c.Assert(rcode, gc.Equals, 0)
	c.Assert(rstdout, gc.Equals, stdout)
	c.Assert(rstderr, gc.Equals, "")
}

func (s *ConstraintsCommandsSuite) TestGetEnvironEmpty(c *gc.C) {
	assertGet(c, "")
}

func (s *ConstraintsCommandsSuite) TestGetEnvironValues(c *gc.C) {
	cons := constraints.Value{CpuCores: uint64p(64)}
	err := s.State.SetEnvironConstraints(cons)
	c.Assert(err, gc.IsNil)
	assertGet(c, "cpu-cores=64\n")
}

func (s *ConstraintsCommandsSuite) TestGetServiceEmpty(c *gc.C) {
	s.AddTestingService(c, "svc", s.AddTestingCharm(c, "dummy"))
	assertGet(c, "", "svc")
}

func (s *ConstraintsCommandsSuite) TestGetServiceValues(c *gc.C) {
	svc := s.AddTestingService(c, "svc", s.AddTestingCharm(c, "dummy"))
	err := svc.SetConstraints(constraints.Value{CpuCores: uint64p(64)})
	c.Assert(err, gc.IsNil)
	assertGet(c, "cpu-cores=64\n", "svc")
}

func (s *ConstraintsCommandsSuite) TestGetFormats(c *gc.C) {
	cons := constraints.Value{CpuCores: uint64p(64), CpuPower: uint64p(0)}
	err := s.State.SetEnvironConstraints(cons)
	c.Assert(err, gc.IsNil)
	assertGet(c, "cpu-cores=64 cpu-power=\n", "--format", "constraints")
	assertGet(c, "cpu-cores: 64\ncpu-power: 0\n", "--format", "yaml")
	assertGet(c, `{"cpu-cores":64,"cpu-power":0}`+"\n", "--format", "json")
}

func assertGetError(c *gc.C, code int, stderr string, args ...string) {
	rcode, rstdout, rstderr := runCmdLine(c, &GetConstraintsCommand{}, args...)
	c.Assert(rcode, gc.Equals, code)
	c.Assert(rstdout, gc.Equals, "")
	c.Assert(rstderr, gc.Matches, "error: "+stderr+"\n")
}

func (s *ConstraintsCommandsSuite) TestGetErrors(c *gc.C) {
	assertGetError(c, 2, `invalid service name "badname-0"`, "badname-0")
	assertGetError(c, 2, `unrecognized args: \["blether"\]`, "goodname", "blether")
	assertGetError(c, 1, `service "missing" not found`, "missing")
}
