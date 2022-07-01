// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/core/instance"
	"github.com/juju/juju/v2/environs/context"
	"github.com/juju/juju/v2/provider/lxd"
)

type instanceSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) TestNewInstance(c *gc.C) {
	inst := lxd.NewInstance(s.Container, s.Env)

	c.Check(lxd.ExposeInstContainer(inst), gc.Equals, s.Container)
	c.Check(lxd.ExposeInstEnv(inst), gc.Equals, s.Env)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestID(c *gc.C) {
	id := s.Instance.Id()

	c.Check(id, gc.Equals, instance.Id("spam"))
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestStatus(c *gc.C) {
	instanceStatus := s.Instance.Status(context.NewEmptyCloudCallContext())

	c.Check(instanceStatus.Message, gc.Equals, "Running")
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestAddresses(c *gc.C) {
	addresses, err := s.Instance.Addresses(context.NewEmptyCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(addresses, jc.DeepEquals, s.Addresses)
}
