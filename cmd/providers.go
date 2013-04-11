package cmd

// Register all the available providers.
// When we import an environment provider implementation
// here, it will register itself with environs.
import (
	_ "launchpad.net/juju-core/environs/ec2"
	_ "launchpad.net/juju-core/environs/maas"
	_ "launchpad.net/juju-core/environs/openstack"
)
