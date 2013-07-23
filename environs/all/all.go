// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

// Register all the available providers.
import (
	//	_ "launchpad.net/juju-core/environs/azure"
	_ "launchpad.net/juju-core/environs/ec2"
	_ "launchpad.net/juju-core/environs/local"
	_ "launchpad.net/juju-core/environs/maas"
	_ "launchpad.net/juju-core/environs/openstack"
)
