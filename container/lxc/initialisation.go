// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/utils"
)

// ltsPackages holds any required packages that may
// install with a different version from their default.
// The installed versions of these packages will be selected
// with targetRelease argument to NewContainerInitialiser.
//
// ltsPackages should include any required packages
// from the cloud archive.
var ltsPackages = []string{
	"lxc",
}

// requiredPackages holds any required packages that
// will always install with their default version.
var requiredPackages = []string{}

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
	return ensureDependencies((*ci).targetRelease)
}

// installLTSPackages issues an AptGetInstall command passing the
// --target-release switch for all of the ltsPackages
func installLTSPackages(targetRelease string) error {
	var args []string
	if targetRelease != "" {
		args = append(args, "--target-release", targetRelease)
	}
	args = append(args, ltsPackages...)
	return utils.AptGetInstall(args...)
}

// ensureDependencies checks the targetRelease and updates the packages
// that are sent to utils.AptGetInstall to include the --target-release
// switch. If targetRelease is an empty string, no switch is passed.
func ensureDependencies(targetRelease string) error {
	var err error
	if err = installLTSPackages(targetRelease); err != nil {
		return err
	}

	if len(requiredPackages) != 0 {
		err = utils.AptGetInstall(requiredPackages...)
	}
	return err
}
