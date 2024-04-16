// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/packaging"
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
func (dep mongoDependency) PackageList(b base.Base) ([]packaging.Package, error) {
	if b.OS != ubuntu {
		return nil, errors.NotSupportedf("installing mongo on base %q", b)
	}

	var (
		snapList []packaging.Package
		pm       = packaging.AptPackageManager
	)

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

	return append(
		packaging.MakePackageList(pm, ""),
		snapList...,
	), nil
}
