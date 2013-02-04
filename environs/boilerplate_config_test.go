package environs_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	_ "launchpad.net/juju-core/environs/ec2"
	_ "launchpad.net/juju-core/environs/openstack"
)

type BoilerplateConfigSuite struct {
}

var _ = Suite(&BoilerplateConfigSuite{})

func (*BoilerplateConfigSuite) TestBoilerPlateGeneration(c *C) {
	boilerplate_text := environs.BoilerplateConfig()
	_, err := environs.ReadEnvironsBytes([]byte(boilerplate_text))
	c.Assert(err, IsNil)
}
