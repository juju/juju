// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/utils"
)

// For LTS releases if NewContainerInitialiser is provided with a
// targetRelease any package in this slice will use --target-release
// during AptGetInstall
var ltsPackages = []string{
	"lxc",
}

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

// installLtsPackages issues an AptGetInstall command passing the
// --target-release switch for all of the ltsPackages
func installLtsPackages(targetRelease string) error {
	packages := []string{
		"--target-release",
		targetRelease,
	}
	packages = append(packages, ltsPackages...)
	return utils.AptGetInstall(packages...)
}

// ensureDependencies checks the targetRelease and updates the packages
// that are sent to utils.AptGetInstall to include the --target-release
// switch. If targetRelease is an empty string, no switch is passed.
func ensureDependencies(targetRelease string) error {
	var packages []string
	var err error
	if targetRelease != "" {
		// if we have a targetRelease, we will run two AptGetInstall commands
		// one with --target-release for ltsPackages and the other without for requiredPackages
		packages = requiredPackages
		err = installLtsPackages(targetRelease)
		if err != nil {
			return err
		}
	} else {
		// if we do not have a targetRelease append the two package slices together
		// and issue a single AptGetInstall command
		packages = append(requiredPackages, ltsPackages...)
	}
	if len(packages) != 0 {
		err = utils.AptGetInstall(packages...)
	}
	return err
}
