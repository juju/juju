// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(dimitern) Disabled on gccgo (PPC64 in particular) due
// to build failures. See bug http://pad.lv/1425788.

// +build !gccgo

package featuretests

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	// TODO(dimitern): Don't import a main package into a library
	// package, pulling in main() along with it.
	"github.com/juju/juju/cmd/envcmd"
	cmdjuju "github.com/juju/juju/cmd/juju"
	"github.com/juju/juju/cmd/juju/service"
	"github.com/juju/juju/constraints"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// cmdJujuSuite tests the connectivity of juju commands.  These tests
// go from the command line, api client, api server, db. The db changes
// are then checked.  Only one test for each command is done here to
// check connectivity.  Exhaustive unit tests are at each layer.
type cmdJujuSuite struct {
	jujutesting.JujuConnSuite
}

func uint64p(val uint64) *uint64 {
	return &val
}

func runCommand(c *gc.C, commands ...string) (*cmd.Context, error) {
	context := testing.Context(c)
	juju := cmdjuju.NewJujuCommand(context)
	if err := testing.InitCommand(juju, commands); err != nil {
		return context, err
	}
	return context, juju.Run(context)
}

func (s *cmdJujuSuite) TestSetConstraints(c *gc.C) {
	_, err := runCommand(c, "set-constraints", "mem=4G", "cpu-power=250")
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.State.EnvironConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, constraints.Value{
		CpuPower: uint64p(250),
		Mem:      uint64p(4096),
	})
}

func (s *cmdJujuSuite) TestGetConstraints(c *gc.C) {
	svc := s.AddTestingService(c, "svc", s.AddTestingCharm(c, "dummy"))
	err := svc.SetConstraints(constraints.Value{CpuCores: uint64p(64)})
	c.Assert(err, jc.ErrorIsNil)

	context, err := runCommand(c, "get-constraints", "svc")
	c.Assert(testing.Stdout(context), gc.Equals, "cpu-cores=64\n")
	c.Assert(testing.Stderr(context), gc.Equals, "")
}

func (s *cmdJujuSuite) TestEnsureAvailability(c *gc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageEnviron},
	})
	ctx, err := runCommand(c, "ensure-availability", "-n", "3")
	c.Assert(err, jc.ErrorIsNil)

	// Machine 0 is demoted because it hasn't reported its presence
	c.Assert(testing.Stdout(ctx), gc.Equals,
		"adding machines: 1, 2, 3\n"+
			"demoting machines 0\n\n")
}

func (s *cmdJujuSuite) TestServiceSetConstraints(c *gc.C) {
	svc := s.AddTestingService(c, "svc", s.AddTestingCharm(c, "dummy"))
	_, err := runCommand(c, "service", "set-constraints", "svc", "mem=4G", "cpu-power=250")
	c.Assert(err, jc.ErrorIsNil)

	cons, err := svc.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, constraints.Value{
		CpuPower: uint64p(250),
		Mem:      uint64p(4096),
	})
}

func (s *cmdJujuSuite) TestServiceGetConstraints(c *gc.C) {
	svc := s.AddTestingService(c, "svc", s.AddTestingCharm(c, "dummy"))
	err := svc.SetConstraints(constraints.Value{CpuCores: uint64p(64)})
	c.Assert(err, jc.ErrorIsNil)

	context, err := runCommand(c, "service", "get-constraints", "svc")
	c.Assert(testing.Stdout(context), gc.Equals, "cpu-cores=64\n")
	c.Assert(testing.Stderr(context), gc.Equals, "")
}

func (s *cmdJujuSuite) TestServiceSet(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "dummy-service", ch)

	_, err := testing.RunCommand(c, envcmd.Wrap(&service.SetCommand{}), "dummy-service",
		"username=hello", "outlook=hello@world.tld")
	c.Assert(err, jc.ErrorIsNil)

	expect := charm.Settings{
		"username": "hello",
		"outlook":  "hello@world.tld",
	}

	settings, err := svc.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, expect)
}

func (s *cmdJujuSuite) TestServiceUnset(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "dummy-service", ch)

	settings := charm.Settings{
		"username": "hello",
		"outlook":  "hello@world.tld",
	}

	err := svc.UpdateConfigSettings(settings)
	c.Assert(err, jc.ErrorIsNil)

	_, err = testing.RunCommand(c, envcmd.Wrap(&service.UnsetCommand{}), "dummy-service", "username")
	c.Assert(err, jc.ErrorIsNil)

	expect := charm.Settings{
		"outlook": "hello@world.tld",
	}
	settings, err = svc.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, expect)
}
