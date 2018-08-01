// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.
// Copied from centos_constants.go (with all pending TODOs)

package config

import (
	"github.com/juju/packaging"
)

const (

	// OpenSUSESourcesFile is the default file which lists all core sources
	// for zypper packages on OpenSUSE.
	OpenSUSESourcesFile = "/etc/zypp/repos.d/repo-oss.repo"
)

// OpenSUSEDefaultPackages is the default package set we'd like installed
// on all OpenSUSE machines.
var OpenSUSEDefaultPackages = append(DefaultPackages, []string{
	"nano", // Not important but useful
	"lsb-release",
}...)

// cloudArchivePackagesOpenSUSE maintains a list of OpenSUSE packages that
// Configurer.IsCloudArchivePackage will reference when determining the
// repository from which to install a package.
var cloudArchivePackagesOpenSUSE = map[string]struct{}{
// TODO (aznashwan, all): if a separate repository for
// OpenSUSE Leap + especially for Juju is to ever occur, please add the relevant
// package listings here.
}

// configureCloudArchiveSourceOpenSUSE is a helper function which returns the
// cloud archive PackageSource and PackagePreferences for the given series for
// OpenSUSE machines.
func configureCloudArchiveSourceOpenSUSE(series string) (packaging.PackageSource, packaging.PackagePreferences) {
	return packaging.PackageSource{}, packaging.PackagePreferences{}
}

// getTargetReleaseSpecifierOpenSUSE returns the specifier that can be passed to
// zypper in order to ensure that it pulls the package from that particular source.
func getTargetReleaseSpecifierOpenSUSE(series string) string {
	return ""
}
