// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package packaging

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/packaging/v2/config"
	"github.com/juju/packaging/v2/manager"
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
	PackageList(series string) ([]Package, error)
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

func (p *Package) appendInstallOptions(opt ...string) {
	p.InstallOptions = strings.TrimSpace(
		fmt.Sprintf("%s %s", p.InstallOptions, strings.Join(opt, " ")),
	)
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
// dependency targeting the provided series.
func InstallDependency(dep Dependency, series string) error {
	pkgManagers := make(map[PackageManagerName]manager.PackageManager)
	pkgConfigurers := make(map[PackageManagerName]config.PackagingConfigurer)
	pkgList, err := dep.PackageList(series)
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

		pkgConfer, exists := pkgConfigurers[pkg.PackageManager]
		if !exists {
			if pkgConfer, err = newPackageConfigurer(pkg.PackageManager, series); err != nil {
				return errors.Annotatef(err, "installing package %q via %q", pkg.Name, pkg.PackageManager)
			}
			pkgConfigurers[pkg.PackageManager] = pkgConfer
		}

		if pkgConfer != nil {
			if config.RequiresBackports(series, pkg.Name) {
				extraOpts := fmt.Sprintf("--target-release %s-backports", series)
				pkg.appendInstallOptions(extraOpts)
			}
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

func newPackageConfigurer(name PackageManagerName, series string) (config.PackagingConfigurer, error) {
	switch name {
	case AptPackageManager:
		return config.NewAptPackagingConfigurer(series), nil
	case YumPackageManager:
		return config.NewYumPackagingConfigurer(series), nil
	case SnapPackageManager:
		return nil, nil
	default:
		return nil, errors.NotImplementedf("%s package configurer", name)
	}
}
