// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package packaging

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/packaging/v3/manager"

	"github.com/juju/juju/core/base"
)

var logger = loggo.GetLogger("juju.packaging")

// PackageManagerName describes a package manager.
type PackageManagerName string

// The list of supported package managers.
const (
	AptPackageManager  PackageManagerName = "apt"
	YumPackageManager  PackageManagerName = "yum"
	SnapPackageManager PackageManagerName = "snap"
)

// Dependency is implemented by objects that can provide a series-specific
// list of packages for installing a particular software dependency.
type Dependency interface {
	PackageList(base.Base) ([]Package, error)
}

// Package encapsulates the information required for installing a package.
type Package struct {
	// The name of the package to install
	Name string

	// Additional options to be passed to the package manager.
	InstallOptions string

	// The package manager to use for installing
	PackageManager PackageManagerName
}

// MakePackageList returns a list of Package instances for each provided
// package name. All package entries share the same package manager name and
// install options.
func MakePackageList(pm PackageManagerName, opts string, packages ...string) []Package {
	var list []Package
	for _, pkg := range packages {
		list = append(list, Package{
			Name:           pkg,
			InstallOptions: opts,
			PackageManager: pm,
		})
	}

	return list
}

// InstallDependency executes the appropriate commands to install the specified
// dependency targeting the provided base.
func InstallDependency(dep Dependency, base base.Base) error {
	pkgManagers := make(map[PackageManagerName]manager.PackageManager)
	pkgList, err := dep.PackageList(base)
	if err != nil {
		return errors.Trace(err)
	}

	for _, pkg := range pkgList {
		pm := pkgManagers[pkg.PackageManager]
		if pm == nil {
			if pm, err = newPackageManager(pkg.PackageManager); err != nil {
				return errors.Annotatef(err, "installing package %q via %q", pkg.Name, pkg.PackageManager)
			}
			pkgManagers[pkg.PackageManager] = pm
		}

		logger.Infof("installing %q via %q", pkg.Name, pkg.PackageManager)

		pkgWithOpts := strings.TrimSpace(fmt.Sprintf("%s %s", pkg.InstallOptions, pkg.Name))
		if err = pm.Install(pkgWithOpts); err != nil {
			return errors.Annotatef(err, "installing package %q via %q", pkg.Name, pkg.PackageManager)
		}
	}

	return nil
}

func newPackageManager(name PackageManagerName) (manager.PackageManager, error) {
	switch name {
	case AptPackageManager:
		return manager.NewAptPackageManager(), nil
	case YumPackageManager:
		return manager.NewYumPackageManager(), nil
	case SnapPackageManager:
		return manager.NewSnapPackageManager(), nil
	default:
		return nil, errors.NotImplementedf("%s package manager", name)
	}
}
