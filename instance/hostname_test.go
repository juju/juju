// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/names"
)

type HostnameSuite struct{}

var _ = gc.Suite(&HostnameSuite{})

const modelUUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"

func (s *HostnameSuite) TestInvalidModelTag(c *gc.C) {
	hostname, err := instance.Hostname(names.NewModelTag("foo"), names.NewMachineTag("2"))
	c.Assert(hostname, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `model ID "foo" is not a valid model`)
}

func (s *HostnameSuite) TestInvalidMachineTag(c *gc.C) {
	hostname, err := instance.Hostname(names.NewModelTag(modelUUID), names.NewMachineTag("foo"))
	c.Assert(hostname, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `machine ID "foo" is not a valid machine`)
}

func (s *HostnameSuite) TestHostname(c *gc.C) {
	hostname, err := instance.Hostname(names.NewModelTag(modelUUID), names.NewMachineTag("2"))
	c.Assert(hostname, gc.Equals, "juju-c3d479-2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HostnameSuite) TestContainerHostname(c *gc.C) {
	hostname, err := instance.Hostname(names.NewModelTag(modelUUID), names.NewMachineTag("2/lxd/4"))
	c.Assert(hostname, gc.Equals, "juju-c3d479-2-lxd-4")
	c.Assert(err, jc.ErrorIsNil)
}
