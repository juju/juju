// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/provider/lxd"
)

type instanceSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) TestNewInstance(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	inst := lxd.NewInstance(s.Container, s.Env)

	c.Check(lxd.ExposeInstContainer(inst), gc.Equals, s.Container)
	c.Check(lxd.ExposeInstEnv(inst), gc.Equals, s.Env)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestID(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	id := s.Instance.Id()

	c.Check(id, gc.Equals, instance.Id("spam"))
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestStatus(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	instanceStatus := s.Instance.Status(envcontext.WithoutCredentialInvalidator(context.Background()))

	c.Check(instanceStatus.Message, gc.Equals, "Running")
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestAddresses(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	addresses, err := s.Instance.Addresses(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(addresses, jc.DeepEquals, s.Addresses)
}
