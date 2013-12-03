// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

// Register all the available providers.
import (
	_ "launchpad.net/juju-core/provider/azure"
	_ "launchpad.net/juju-core/provider/ec2"
	_ "launchpad.net/juju-core/provider/local"
	_ "launchpad.net/juju-core/provider/maas"
	_ "launchpad.net/juju-core/provider/null"
	_ "launchpad.net/juju-core/provider/openstack"
	//_ "launchpad.net/juju-core/provider/joyent"
)
