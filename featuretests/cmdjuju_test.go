// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(dimitern) Disabled on gccgo (PPC64 in particular) due
// to build failures. See bug http://pad.lv/1425788.

// +build !gccgo

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/service"
	"github.com/juju/juju/constraints"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
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

func (s *cmdJujuSuite) TestSetConstraints(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&common.SetConstraintsCommand{}),
		"mem=4G", "cpu-power=250")
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

	context, err := testing.RunCommand(c, envcmd.Wrap(&common.GetConstraintsCommand{}), "svc")
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

func (s *cmdJujuSuite) TestServiceGet(c *gc.C) {
	expected := `charm: dummy
service: dummy-service
settings:
  outlook:
    default: true
    description: No default outlook.
    type: string
  skill-level:
    default: true
    description: A number indicating skill.
    type: int
  title:
    default: true
    description: A descriptive title used for the service.
    type: string
    value: My Title
  username:
    default: true
    description: The name of the initial account (given admin permissions).
    type: string
    value: admin001
`
	ch := s.AddTestingCharm(c, "dummy")
	s.AddTestingService(c, "dummy-service", ch)

	context, err := testing.RunCommand(c, envcmd.Wrap(&service.GetCommand{}), "dummy-service")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}
