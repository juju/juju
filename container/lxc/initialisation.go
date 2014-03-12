// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"fmt"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/utils"
)

var requiredPackages = []string{
	"lxc",
}

type containerInitialiser struct {
	targetRelease string
}

// containerInitialiser implements container.Initialiser.
var _ container.Initialiser = (*containerInitialiser)(nil)

// NewContainerInitialiser returns an instance used to perform the steps
// required to allow a host machine to run a LXC container.
func NewContainerInitialiser(targetRelease string) container.Initialiser {
	return &containerInitialiser{targetRelease}
}

// Initialise is specified on the container.Initialiser interface.
func (ci *containerInitialiser) Initialise() error {
	return ensureDependencies()
}

// updateTargetRelease will overwrite the lxc package in the
// requiredPackages slice if containerInitialiser.targetRelease
// is set to a value other than empty string.
func (ci *containerInitialiser) updateTargetRelease() {
	if ci.targetRelease != "" {
		pkg := requiredPackages[0]
		requiredPackages[0] = fmt.Sprintf("--target-release '%s' %s'", ci.targetRelease, pkg)
	}
}

func ensureDependencies() error {
	return utils.AptGetInstall(requiredPackages...)
}
