// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/names"
)

type NamespaceSuite struct{}

var _ = gc.Suite(&NamespaceSuite{})

const modelUUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"

func (s *NamespaceSuite) TestInvalidModelTag(c *gc.C) {
	ns, err := instance.NewNamespace("foo")
	c.Assert(ns, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `model ID "foo" is not a valid model`)
}

func (s *NamespaceSuite) newNamespace(c *gc.C) instance.Namespace {
	ns, err := instance.NewNamespace(modelUUID)
	c.Assert(err, jc.ErrIsNil)
	return ns
}

func (s *NamespaceSuite) TestInvalidMachineTag(c *gc.C) {
	ns := s.newNamespace()
	hostname, err := ns.Hostname("foo")
	c.Assert(hostname, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `machine ID "foo" is not a valid machine`)
}

func (s *NamespaceSuite) TestHostname(c *gc.C) {
	ns := s.newNamespace()
	hostname, err := ns.Hostname("2")
	c.Assert(hostname, gc.Equals, "juju-c3d479-2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NamespaceSuite) TestContainerHostname(c *gc.C) {
	ns := s.newNamespace()
	hostname, err := ns.Hostname("2/lxd/4")
	c.Assert(hostname, gc.Equals, "juju-c3d479-2-lxd-4")
	c.Assert(err, jc.ErrorIsNil)
}
