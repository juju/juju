// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	_ "launchpad.net/juju-core/provider/ec2"
	_ "launchpad.net/juju-core/provider/openstack"
)

type BoilerplateConfigSuite struct {
}

var _ = Suite(&BoilerplateConfigSuite{})

func (*BoilerplateConfigSuite) TestBoilerPlateGeneration(c *C) {
	defer config.SetJujuHome(config.SetJujuHome(c.MkDir()))
	boilerplate_text := environs.BoilerplateConfig()
	_, err := environs.ReadEnvironsBytes([]byte(boilerplate_text))
	c.Assert(err, IsNil)
}
