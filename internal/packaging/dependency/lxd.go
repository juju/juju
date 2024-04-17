// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/packaging"
)

var ubuntu = "ubuntu"

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
func (dep lxdDependency) PackageList(b base.Base) ([]packaging.Package, error) {
	if b.OS != ubuntu {
		return nil, errors.NotSupportedf("LXD containers on base %q", b)
	}

	var pkg packaging.Package

	if dep.snapChannel == "" {
		return nil, errors.NotValidf("snap channel for lxd dependency")
	}

	pkg.Name = "lxd"
	pkg.InstallOptions = fmt.Sprintf("--classic --channel %s", dep.snapChannel)
	pkg.PackageManager = packaging.SnapPackageManager

	return []packaging.Package{pkg}, nil
}
