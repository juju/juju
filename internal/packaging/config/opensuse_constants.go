// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.
// Copied from centos_constants.go (with all pending TODOs)

package config

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
