// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/provider/lxd"
)

type instanceSuite struct {
	lxd.BaseSuite
}

var _ = tc.Suite(&instanceSuite{})

func (s *instanceSuite) TestNewInstance(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	inst := lxd.NewInstance(s.Container, s.Env)

	c.Check(lxd.ExposeInstContainer(inst), tc.Equals, s.Container)
	c.Check(lxd.ExposeInstEnv(inst), tc.Equals, s.Env)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestID(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	id := s.Instance.Id()

	c.Check(id, tc.Equals, instance.Id("spam"))
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestStatus(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	instanceStatus := s.Instance.Status(context.Background())

	c.Check(instanceStatus.Message, tc.Equals, "Running")
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestAddresses(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	addresses, err := s.Instance.Addresses(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(addresses, jc.DeepEquals, s.Addresses)
}
