// Copyright 2013 - 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"bytes"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	// TODO(dimitern): Don't ever import "." unless there's a GOOD
	// reason to do it.
	. "github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/testing"
)

type ConstraintsCommandsSuite struct {
	testing.FakeJujuHomeSuite
	fake *fakeConstraintsClient
}

var _ = gc.Suite(&ConstraintsCommandsSuite{})

type fakeConstraintsClient struct {
	err      error
	envCons  constraints.Value
	servCons map[string]constraints.Value
}

func (f *fakeConstraintsClient) addTestingService(name string) {
	f.servCons[name] = constraints.Value{}
}

func (f *fakeConstraintsClient) Close() error {
	return nil
}

func (f *fakeConstraintsClient) GetEnvironmentConstraints() (constraints.Value, error) {
	return f.envCons, nil
}

func (f *fakeConstraintsClient) GetServiceConstraints(name string) (constraints.Value, error) {
	if !names.IsValidService(name) {
		return constraints.Value{}, errors.Errorf("%q is not a valid service name", name)
	}

	cons, ok := f.servCons[name]
	if !ok {
		return constraints.Value{}, errors.NotFoundf("service %q", name)
	}

	return cons, nil
}

func (f *fakeConstraintsClient) SetEnvironmentConstraints(cons constraints.Value) error {
	if f.err != nil {
		return f.err
	}

	f.envCons = cons
	return nil
}

func (f *fakeConstraintsClient) SetServiceConstraints(name string, cons constraints.Value) error {
	if f.err != nil {
		return f.err
	}

	if !names.IsValidService(name) {
		return errors.Errorf("%q is not a valid service name", name)
	}

	_, ok := f.servCons[name]
	if !ok {
		return errors.NotFoundf("service %q", name)
	}

	f.servCons[name] = cons
	return nil
}

func runCmdLine(c *gc.C, com cmd.Command, args ...string) (code int, stdout, stderr string) {
	ctx := testing.Context(c)
	code = cmd.Main(com, ctx, args)
	stdout = ctx.Stdout.(*bytes.Buffer).String()
	stderr = ctx.Stderr.(*bytes.Buffer).String()
	c.Logf("args:   %#v\ncode:   %d\nstdout: %q\nstderr: %q", args, code, stdout, stderr)
	return
}

func uint64p(val uint64) *uint64 {
	return &val
}

func (s *ConstraintsCommandsSuite) assertSet(c *gc.C, args ...string) {
	command := NewSetConstraintsCommand(s.fake)
	rcode, rstdout, rstderr := runCmdLine(c, envcmd.Wrap(command), args...)

	c.Assert(rcode, gc.Equals, 0)
	c.Assert(rstdout, gc.Equals, "")
	c.Assert(rstderr, gc.Equals, "")
}

func (s *ConstraintsCommandsSuite) assertSetBlocked(c *gc.C, args ...string) {
	command := NewSetConstraintsCommand(s.fake)
	rcode, _, _ := runCmdLine(c, envcmd.Wrap(command), args...)

	c.Assert(rcode, gc.Equals, 1)

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*To unblock changes.*")
}

func (s *ConstraintsCommandsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.fake = &fakeConstraintsClient{servCons: make(map[string]constraints.Value)}
}

func (s *ConstraintsCommandsSuite) TestSetEnviron(c *gc.C) {
	// Set constraints.
	s.assertSet(c, "mem=4G", "cpu-power=250")
	cons, err := s.fake.GetEnvironmentConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, constraints.Value{
		CpuPower: uint64p(250),
		Mem:      uint64p(4096),
	})

	// Clear constraints.
	s.assertSet(c)
	cons, err = s.fake.GetEnvironmentConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)
}

func (s *ConstraintsCommandsSuite) TestBlockSetEnviron(c *gc.C) {
	// Block operation
	s.fake.err = common.ErrOperationBlocked("TestBlockSetEnviron")
	// Set constraints.
	s.assertSetBlocked(c, "mem=4G", "cpu-power=250")
}

func (s *ConstraintsCommandsSuite) TestSetService(c *gc.C) {
	s.fake.addTestingService("svc")

	// Set constraints.
	s.assertSet(c, "-s", "svc", "mem=4G", "cpu-power=250")
	cons := s.fake.servCons["svc"]
	c.Assert(cons, gc.DeepEquals, constraints.Value{
		CpuPower: uint64p(250),
		Mem:      uint64p(4096),
	})

	// Clear constraints.
	s.assertSet(c, "-s", "svc")
	cons = s.fake.servCons["svc"]
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)
}

func (s *ConstraintsCommandsSuite) TestBlockSetService(c *gc.C) {
	s.fake.addTestingService("svc")

	// Block operation
	s.fake.err = common.ErrOperationBlocked("TestBlockSetService")
	// Set constraints.
	s.assertSetBlocked(c, "-s", "svc", "mem=4G", "cpu-power=250")
}

func (s *ConstraintsCommandsSuite) assertSetError(c *gc.C, code int, stderr string, args ...string) {
	command := NewSetConstraintsCommand(s.fake)
	rcode, rstdout, rstderr := runCmdLine(c, envcmd.Wrap(command), args...)
	c.Assert(rcode, gc.Equals, code)
	c.Assert(rstdout, gc.Equals, "")
	c.Assert(rstderr, gc.Matches, "error: "+stderr+"\n")
}

func (s *ConstraintsCommandsSuite) TestSetErrors(c *gc.C) {
	s.assertSetError(c, 2, `invalid service name "badname-0"`, "-s", "badname-0")
	s.assertSetError(c, 2, `malformed constraint "="`, "=")
	s.assertSetError(c, 2, `malformed constraint "="`, "-s", "s", "=")
	s.assertSetError(c, 1, `service "missing" not found`, "-s", "missing")
}

func (s *ConstraintsCommandsSuite) assertGet(c *gc.C, stdout string, args ...string) {
	command := NewGetConstraintsCommand(s.fake)
	rcode, rstdout, rstderr := runCmdLine(c, envcmd.Wrap(command), args...)
	c.Assert(rcode, gc.Equals, 0)
	c.Assert(rstdout, gc.Equals, stdout)
	c.Assert(rstderr, gc.Equals, "")
}

func (s *ConstraintsCommandsSuite) TestGetEnvironEmpty(c *gc.C) {
	s.assertGet(c, "")
}

func (s *ConstraintsCommandsSuite) TestGetEnvironValues(c *gc.C) {
	cons := constraints.Value{CpuCores: uint64p(64)}
	s.fake.SetEnvironmentConstraints(cons)
	s.assertGet(c, "cpu-cores=64\n")
}

func (s *ConstraintsCommandsSuite) TestGetServiceEmpty(c *gc.C) {
	s.fake.addTestingService("svc")
	s.assertGet(c, "", "svc")
}

func (s *ConstraintsCommandsSuite) TestGetServiceValues(c *gc.C) {
	s.fake.addTestingService("svc")
	s.fake.SetServiceConstraints("svc", constraints.Value{CpuCores: uint64p(64)})
	s.assertGet(c, "cpu-cores=64\n", "svc")
}

func (s *ConstraintsCommandsSuite) TestGetFormats(c *gc.C) {
	cons := constraints.Value{CpuCores: uint64p(64), CpuPower: uint64p(0)}
	s.fake.SetEnvironmentConstraints(cons)
	s.assertGet(c, "cpu-cores=64 cpu-power=\n", "--format", "constraints")
	s.assertGet(c, "cpu-cores: 64\ncpu-power: 0\n", "--format", "yaml")
	s.assertGet(c, `{"cpu-cores":64,"cpu-power":0}`+"\n", "--format", "json")
}

func (s *ConstraintsCommandsSuite) assertGetError(c *gc.C, code int, stderr string, args ...string) {
	command := NewGetConstraintsCommand(s.fake)
	rcode, rstdout, rstderr := runCmdLine(c, envcmd.Wrap(command), args...)
	c.Assert(rcode, gc.Equals, code)
	c.Assert(rstdout, gc.Equals, "")
	c.Assert(rstderr, gc.Matches, "error: "+stderr+"\n")
}

func (s *ConstraintsCommandsSuite) TestGetErrors(c *gc.C) {
	s.assertGetError(c, 2, `invalid service name "badname-0"`, "badname-0")
	s.assertGetError(c, 2, `unrecognized args: \["blether"\]`, "goodname", "blether")
	s.assertGetError(c, 1, `service "missing" not found`, "missing")
}
