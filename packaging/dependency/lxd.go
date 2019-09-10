// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"github.com/juju/errors"
	"github.com/juju/juju/packaging"
)

const blankSeries = ""

// LXD returns a dependency instance for installing lxd support
func LXD() packaging.Dependency {
	return lxdDependency{}
}

type lxdDependency struct{}

// PackageList implements packaging.Dependency.
func (lxdDependency) PackageList(series string) ([]packaging.Package, error) {
	var pkg packaging.Package

	switch series {
	case "centos7", "opensuseleap", "precise":
		return nil, errors.NotSupportedf("LXD containers on series %q", series)
	case "trusty", "xenial", "bionic", blankSeries:
		pkg.Name = "lxd"
		pkg.PackageManager = packaging.AptPackageManager
	default: // Use snaps for cosmic and beyond
		pkg.Name = "lxd"
		pkg.InstallOptions = "--classic"
		pkg.PackageManager = packaging.SnapPackageManager
	}

	return []packaging.Package{pkg}, nil
}
