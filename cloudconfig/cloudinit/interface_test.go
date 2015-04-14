// Copyright 2011, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"github.com/juju/juju/cloudconfig/cloudinit"
)

var _ cloudinit.CloudConfig = (*cloudinit.UbuntuCloudConfig)(nil)
