// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"github.com/juju/utils/apt"

	"github.com/juju/juju/container"
)

var requiredPackages = []string{
	"lxc",
	"cloud-image-utils",
}

type containerInitialiser struct {
	series string
}

// containerInitialiser implements container.Initialiser.
var _ container.Initialiser = (*containerInitialiser)(nil)

// NewContainerInitialiser returns an instance used to perform the steps
// required to allow a host machine to run a LXC container.
func NewContainerInitialiser(series string) container.Initialiser {
	return &containerInitialiser{series}
}

// Initialise is specified on the container.Initialiser interface.
func (ci *containerInitialiser) Initialise() error {
	return ensureDependencies(ci.series)
}

// ensureDependencies creates a set of install packages using AptGetPreparePackages
// and runs each set of packages through AptGetInstall
func ensureDependencies(series string) error {
	var err error
	aptGetInstallCommandList := apt.GetPreparePackages(requiredPackages, series)
	for _, commands := range aptGetInstallCommandList {
		err = apt.GetInstall(commands...)
		if err != nil {
			return err
		}
	}
	return err
}
