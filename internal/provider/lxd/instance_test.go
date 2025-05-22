// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/provider/lxd"
)

type instanceSuite struct {
	lxd.BaseSuite
}

func TestInstanceSuite(t *testing.T) {
	tc.Run(t, &instanceSuite{})
}

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

	instanceStatus := s.Instance.Status(c.Context())

	c.Check(instanceStatus.Message, tc.Equals, "Running")
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestAddresses(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	addresses, err := s.Instance.Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(addresses, tc.DeepEquals, s.Addresses)
}
