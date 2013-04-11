package all

// Register all the available providers.
import (
	_ "launchpad.net/juju-core/environs/ec2"
	_ "launchpad.net/juju-core/environs/maas"
	_ "launchpad.net/juju-core/environs/openstack"
)
