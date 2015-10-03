// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/environs/tags"
)

// The metadata keys used when creating new instances.
const (
	metadataKeyIsState = tags.JujuEnv
	// This is defined by the cloud-init code:
	// http://bazaar.launchpad.net/~cloud-init-dev/cloud-init/trunk/view/head:/cloudinit/sources/
	// http://cloudinit.readthedocs.org/en/latest/
	// Also see https://github.com/lxc/lxd/blob/master/specs/configuration.md.
	metadataKeyCloudInit = "user-data"
	//metadataKeyCloudInit = "user.user-data"
)

// Common metadata values used when creating new instances.
const (
	metadataValueTrue  = "true"
	metadataValueFalse = "false"
)

var (
	logger = loggo.GetLogger("juju.provider.lxd")
)
