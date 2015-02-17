// Copyright 2011, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import "github.com/juju/juju/cloudinit"

var _ cloudinit.Config = (*cloudinit.UbuntuCloudConfig)(nil)
