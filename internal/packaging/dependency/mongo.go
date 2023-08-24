// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/packaging"
)

type mongoDependency struct {
	snapChannel string
}

// Mongo returns a dependency for installing mongo server using the specified
// NUMA settings and snap channel preferences (applies to focal or later).
func Mongo(snapChannel string) packaging.Dependency {
	return mongoDependency{
		snapChannel: snapChannel,
	}
}

// PackageList implements packaging.Dependency.
func (dep mongoDependency) PackageList(series string) ([]packaging.Package, error) {
	var (
		snapList []packaging.Package
		pm       = packaging.AptPackageManager
	)

	switch series {
	case "centos7":
		return nil, errors.NotSupportedf("installing mongo on series %q", series)
	default:
		if dep.snapChannel == "" {
			return nil, errors.NotValidf("snap channel for mongo dependency")
		}

		snapList = append(
			snapList,
			packaging.Package{
				Name:           "juju-db",
				InstallOptions: fmt.Sprintf("--channel %s", dep.snapChannel),
				PackageManager: packaging.SnapPackageManager,
			},
		)
	}

	return append(
		packaging.MakePackageList(pm, ""),
		snapList...,
	), nil
}
