// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"github.com/juju/errors"
	"github.com/juju/juju/packaging"
)

type mongoDependency struct {
	useNUMA bool
}

// Mongo returns a dependency for installing mongo server using the specified
// NUMA settings.
func Mongo(useNUMA bool) packaging.Dependency {
	return mongoDependency{useNUMA: useNUMA}
}

// PackageList implements packaging.Dependency.
func (dep mongoDependency) PackageList(series string) ([]packaging.Package, error) {
	var pkgList []string
	var pm = packaging.AptPackageManager

	if dep.useNUMA {
		pkgList = append(pkgList, "numactl")
	}

	switch series {
	case "centos7", "opensuseleap", "precise":
		return nil, errors.NotSupportedf("installing mongo on series %q", series)
	case "trusty":
		pkgList = append(pkgList, "juju-mongodb")
	case "xenial":
		// The tools package provides mongodump and friends.
		pkgList = append(pkgList, "juju-mongodb3.2", "juju-mongo-tools3.2")
	default: // Bionic and newer
		pkgList = append(pkgList, "mongodb-server-core", "mongodb-clients")
	}

	return packaging.MakePackageList(pm, "", pkgList...), nil
}
