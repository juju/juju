// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"strings"

	"github.com/juju/utils/packaging/config"
	"github.com/juju/utils/packaging/manager"

	"github.com/juju/juju/container"
	"github.com/juju/juju/version"
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

// getPackageManager is a helper function which returns the
// package manager implementation for the current system.
func getPackageManager() (manager.PackageManager, error) {
	return manager.NewPackageManager(version.Current.Series)
}

// getPackagingConfigurer is a helper function which returns the
// packaging configuration manager for the current system.
func getPackagingConfigurer() (config.PackagingConfigurer, error) {
	return config.NewPackagingConfigurer(version.Current.Series)
}

// ensureDependencies creates a set of install packages using
// apt.GetPreparePackages and runs each set of packages through
// apt.GetInstall.
func ensureDependencies(series string) error {
	pacman, err := getPackageManager()
	if err != nil {
		return err
	}
	pacconfer, err := getPackagingConfigurer()
	if err != nil {
		return err
	}

	for _, pack := range requiredPackages {
		pkg := pack
		if config.SeriesRequiresCloudArchiveTools(version.Current.Series) &&
			pacconfer.IsCloudArchivePackage(pack) {
			pkg = strings.Join(pacconfer.ApplyCloudArchiveTarget(pack), " ")
		}

		if err := pacman.Install(pkg); err != nil {
			return err
		}
	}

	return err
}
