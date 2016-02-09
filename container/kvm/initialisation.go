// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"github.com/juju/utils/packaging/manager"
	"github.com/juju/utils/series"

	"github.com/juju/juju/container"
)

var requiredPackages = []string{
	"uvtool-libvirt",
	"uvtool",
}

type containerInitialiser struct{}

// containerInitialiser implements container.Initialiser.
var _ container.Initialiser = (*containerInitialiser)(nil)

// NewContainerInitialiser returns an instance used to perform the steps
// required to allow a host machine to run a KVM container.
func NewContainerInitialiser() container.Initialiser {
	return &containerInitialiser{}
}

// Initialise is specified on the container.Initialiser interface.
func (ci *containerInitialiser) Initialise() error {
	return ensureDependencies()
}

// getPackageManager is a helper function which returns the
// package manager implementation for the current system.
func getPackageManager() (manager.PackageManager, error) {
	return manager.NewPackageManager(series.HostSeries())
}

func ensureDependencies() error {
	pacman, err := getPackageManager()
	if err != nil {
		return err
	}

	for _, pack := range requiredPackages {
		if err := pacman.Install(pack); err != nil {
			return err
		}
	}

	return nil
}
