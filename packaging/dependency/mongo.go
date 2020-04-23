// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/packaging"
)

type mongoDependency struct {
	useNUMA     bool
	snapChannel string
}

// Mongo returns a dependency for installing mongo server using the specified
// NUMA settings and snap channel preferences (applies to focal or later).
func Mongo(useNUMA bool, snapChannel string) packaging.Dependency {
	return mongoDependency{
		useNUMA:     useNUMA,
		snapChannel: snapChannel,
	}
}

// PackageList implements packaging.Dependency.
func (dep mongoDependency) PackageList(series string) ([]packaging.Package, error) {
	var (
		aptPkgList []string
		snapList   []packaging.Package
		pm         = packaging.AptPackageManager
	)

	if dep.useNUMA {
		aptPkgList = append(aptPkgList, "numactl")
	}

	switch series {
	case "centos7", "opensuseleap", "precise":
		return nil, errors.NotSupportedf("installing mongo on series %q", series)
	case "trusty":
		aptPkgList = append(aptPkgList, "juju-mongodb")
	case "xenial":
		// The tools package provides mongodump and friends.
		aptPkgList = append(aptPkgList, "juju-mongodb3.2", "juju-mongo-tools3.2")
	case "bionic", "cosmic", "disco", "eoan":
		aptPkgList = append(aptPkgList, "mongodb-server-core", "mongodb-clients")
	default: // Focal and beyond always use snaps
		if dep.snapChannel == "" {
			return nil, errors.NotValidf("snap channel for mongo dependency")
		}

		snapList = append(
			snapList,
			packaging.Package{
				Name:           "core",
				PackageManager: packaging.SnapPackageManager,
			},
			packaging.Package{
				Name:           "juju-db",
				InstallOptions: fmt.Sprintf("--channel %s", dep.snapChannel),
				PackageManager: packaging.SnapPackageManager,
			},
		)
	}

	return append(
		packaging.MakePackageList(pm, "", aptPkgList...),
		snapList...,
	), nil
}
