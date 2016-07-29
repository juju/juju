// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

// The config package defines an interface which returns packaging-related
// configuration options and operations depending on the desired
// package-management system.
package config

import (
	"github.com/juju/utils/packaging"
)

// PackagingConfigurer is an interface which handles various packaging-related configuration
// functions for the specific distribution it represents.
type PackagingConfigurer interface {
	// DefaultPackages returns a list of default packages whcih should be
	// installed the vast majority of cases on any specific machine
	DefaultPackages() []string

	// GetPackageNameForSeries returns the equivalent package name of the
	// specified package for the given series or an error if no mapping
	// for it exists.
	GetPackageNameForSeries(pack string, series string) (string, error)

	// IsCloudArchivePackage signals whether the given package is a
	// cloud archive package and thus should be set as such.
	IsCloudArchivePackage(pack string) bool

	// ApplyCloudArchiveTarget returns the package with the required target
	// release bits preceding it.
	ApplyCloudArchiveTarget(pack string) []string

	// RenderSource returns the os-specific full file contents
	// of a given PackageSource.
	RenderSource(src packaging.PackageSource) (string, error)

	// RenderPreferences returns the os-specific full file contents of a given
	// set of PackagePreferences.
	RenderPreferences(prefs packaging.PackagePreferences) (string, error)
}

func NewPackagingConfigurer(series string) (PackagingConfigurer, error) {
	switch series {
	// TODO (aznashwan): find a more deterministic way of selection here
	// without importing version from core.
	case "centos7":
		return NewYumPackagingConfigurer(series), nil
	default:
		return NewAptPackagingConfigurer(series), nil
	}

	return nil, nil
}

// NewAptPackagingConfigurer returns a PackagingConfigurer for apt-based systems.
func NewAptPackagingConfigurer(series string) PackagingConfigurer {
	return &aptConfigurer{&baseConfigurer{
		series:               series,
		defaultPackages:      UbuntuDefaultPackages,
		cloudArchivePackages: cloudArchivePackagesUbuntu,
	}}
}

// NewYumPackagingConfigurer returns a PackagingConfigurer for yum-based systems.
func NewYumPackagingConfigurer(series string) PackagingConfigurer {
	return &yumConfigurer{&baseConfigurer{
		series:               series,
		defaultPackages:      CentOSDefaultPackages,
		cloudArchivePackages: cloudArchivePackagesCentOS,
	}}
}
