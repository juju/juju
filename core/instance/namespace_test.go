// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/instance"
)

type NamespaceSuite struct{}

var _ = tc.Suite(&NamespaceSuite{})

const modelUUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"

func (s *NamespaceSuite) TestInvalidModelTag(c *tc.C) {
	ns, err := instance.NewNamespace("foo")
	c.Assert(ns, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, `model UUID "foo" not valid`)
}

func (s *NamespaceSuite) newNamespace(c *tc.C) instance.Namespace {
	ns, err := instance.NewNamespace(modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	return ns
}

func (s *NamespaceSuite) TestInvalidMachineTag(c *tc.C) {
	ns := s.newNamespace(c)
	hostname, err := ns.Hostname("foo")
	c.Assert(hostname, tc.Equals, "")
	c.Assert(err, tc.ErrorMatches, `machine ID "foo" is not a valid machine`)
}

func (s *NamespaceSuite) TestHostname(c *tc.C) {
	ns := s.newNamespace(c)
	hostname, err := ns.Hostname("2")
	c.Assert(hostname, tc.Equals, "juju-c3d479-2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NamespaceSuite) TestContainerHostname(c *tc.C) {
	ns := s.newNamespace(c)
	hostname, err := ns.Hostname("2/lxd/4")
	c.Assert(hostname, tc.Equals, "juju-c3d479-2-lxd-4")
	c.Assert(err, jc.ErrorIsNil)
}
