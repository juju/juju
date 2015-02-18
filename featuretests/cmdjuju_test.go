// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	cmdjuju "github.com/juju/juju/cmd/juju"
	"github.com/juju/juju/constraints"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

// CommonSuite tests the connectivity of all the common commands.
// These tests go from the command line, api client, api server, db. The db
// changes are then checked.  Only one test for each command is done here to
// check connectivity.  Exhaustive unit tests are at each layer.
type CommonSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&CommonSuite{})

func uint64p(val uint64) *uint64 {
	return &val
}

func runCommonCommand(c *gc.C, commands ...string) (*cmd.Context, error) {
	context := testing.Context(c)
	juju := cmdjuju.NewJujuCommand(context)
	if err := testing.InitCommand(juju, commands); err != nil {
		return context, err
	}
	return context, juju.Run(context)
}

func (s *CommonSuite) TestSetConstraints(c *gc.C) {
	_, err := runCommonCommand(c, "set-constraints", "mem=4G", "cpu-power=250")
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.State.EnvironConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, constraints.Value{
		CpuPower: uint64p(250),
		Mem:      uint64p(4096),
	})
}

func (s *CommonSuite) TestGetConstraints(c *gc.C) {
	svc := s.AddTestingService(c, "svc", s.AddTestingCharm(c, "dummy"))
	err := svc.SetConstraints(constraints.Value{CpuCores: uint64p(64)})
	c.Assert(err, jc.ErrorIsNil)

	context, err := runCommonCommand(c, "get-constraints", "svc")
	c.Assert(testing.Stdout(context), gc.Equals, "cpu-cores=64\n")
	c.Assert(testing.Stderr(context), gc.Equals, "")
}
