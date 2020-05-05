// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/packaging"
)

const blankSeries = ""

// LXD returns a dependency instance for installing lxd support using the
// specified channel preferences (applies to cosmic or later).
func LXD(snapChannel string) packaging.Dependency {
	return lxdDependency{
		snapChannel: snapChannel,
	}
}

type lxdDependency struct {
	snapChannel string
}

// PackageList implements packaging.Dependency.
func (dep lxdDependency) PackageList(series string) ([]packaging.Package, error) {
	var pkg packaging.Package

	switch series {
	case "centos7", "opensuseleap", "precise":
		return nil, errors.NotSupportedf("LXD containers on series %q", series)
	case "trusty", "xenial", "bionic", blankSeries:
		pkg.Name = "lxd"
		pkg.PackageManager = packaging.AptPackageManager
	default: // Use snaps for cosmic and beyond
		if dep.snapChannel == "" {
			return nil, errors.NotValidf("snap channel for lxd dependency")
		}

		pkg.Name = "lxd"
		pkg.InstallOptions = fmt.Sprintf("--classic --channel %s", dep.snapChannel)
		pkg.PackageManager = packaging.SnapPackageManager
	}

	return []packaging.Package{pkg}, nil
}
