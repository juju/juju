// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

// The manager package defines an interface which can carry out numerous
// package-management related operations on the local system and the respective
// implementations on apt and yum-based systems.
package manager

import (
	"github.com/juju/utils/packaging/commands"
	"github.com/juju/utils/proxy"
)

// PackageManager is the interface which carries out various
// package-management related work.
type PackageManager interface {
	// InstallPrerequisite runs the command which installs the prerequisite
	// package which provides repository management functionalityes.
	InstallPrerequisite() error

	// Update runs the command to update the local package list.
	Update() error

	// Upgrade runs the command which issues an upgrade on all packages
	// with available newer versions.
	Upgrade() error

	// Install runs a *single* command that installs the given package(s).
	Install(packs ...string) error

	// Remove runs a *single* command that removes the given package(s).
	Remove(packs ...string) error

	// Purge runs the command that removes the given package(s) along
	// with any associated config files.
	Purge(packs ...string) error

	// Search runs the command that determines whether the given package is
	// available for installation from the currently configured repositories.
	Search(pack string) (bool, error)

	// IsInstalled runs the command which determines whether or not the
	// given package is currently installed on the system.
	IsInstalled(pack string) bool

	// AddRepository runs the command that adds a repository to the
	// list of available repositories.
	// NOTE: requires the prerequisite package whose installation command
	// is done by running InstallPrerequisite().
	AddRepository(repo string) error

	// RemoveRepository runs the command that removes a given
	// repository from the list of available repositories.
	// NOTE: requires the prerequisite package whose installation command
	// is done by running InstallPrerequisite().
	RemoveRepository(repo string) error

	// Cleanup runs the command that cleans up all orphaned packages,
	// left-over files and previously-cached packages.
	Cleanup() error

	// GetProxySettings returns the curretly-configured package manager proxy.
	GetProxySettings() (proxy.Settings, error)

	// SetProxy runs the commands to set the given proxy parameters for the
	// package management system.
	SetProxy(settings proxy.Settings) error
}

// NewPackageManager returns the appropriate PackageManager implementation
// based on the provided series.
func NewPackageManager(series string) (PackageManager, error) {
	// TODO (aznashwan): find a more deterministic way of filtering out
	// release series without importing version from core.
	switch series {
	case "centos7":
		return NewYumPackageManager(), nil
	default:
		return NewAptPackageManager(), nil
	}

	return nil, nil
}

// NewAptPackageManager returns a PackageManager for apt-based systems.
func NewAptPackageManager() PackageManager {
	return &apt{basePackageManager{commands.NewAptPackageCommander()}}
}

// NewYumPackageManager returns a PackageManager for yum-based systems.
func NewYumPackageManager() PackageManager {
	return &yum{basePackageManager{commands.NewYumPackageCommander()}}
}
