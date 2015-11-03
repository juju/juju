// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

// Register all the available providers.
import (
	_ "github.com/juju/juju/provider/azure"
	_ "github.com/juju/juju/provider/cloudsigma"
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/gce"
	_ "github.com/juju/juju/provider/joyent"
	_ "github.com/juju/juju/provider/local"
	_ "github.com/juju/juju/provider/maas"
	_ "github.com/juju/juju/provider/manual"
	_ "github.com/juju/juju/provider/openstack"
	_ "github.com/juju/juju/provider/vsphere"
)
