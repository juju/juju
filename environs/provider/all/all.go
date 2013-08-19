// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

// Register all the available providers.
import (
	_ "launchpad.net/juju-core/environs/provider/azure"
	_ "launchpad.net/juju-core/environs/provider/ec2"
	_ "launchpad.net/juju-core/environs/provider/local"
	_ "launchpad.net/juju-core/environs/provider/maas"
	_ "launchpad.net/juju-core/environs/provider/openstack"
)
