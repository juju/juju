// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/utils"
)

// Functions in this package are here to help us with post-upgrade installing,
// etc. It is used to validate the system status after we have upgraded.

var validationLogger = loggo.GetLogger("juju.jujud.validation")

// EnsureWeHaveLXC checks if we have lxc installed, and installs it if we
// don't. Juju 1.11 added the ability to deploy into LXC containers, and uses
// functionality from the lxc package in order to do so. Juju 1.10 did not
// install lxc, so we ensure it is installed.
// See http://bugs.launchpad.net/bug/1199913
func EnsureWeHaveLXC() error {
	manager := lxc.NewContainerManager("lxc-test")
	if _, err := manager.ListContainers(); err == nil {
		validationLogger.Debugf("found lxc, not installing")
		// We already have it, nothing more to do
		return nil
	} else {
		validationLogger.Debugf("got error looking for lxc, attempting to install: %v", err)
	}
	return utils.AptGetInstall("lxc")
}
