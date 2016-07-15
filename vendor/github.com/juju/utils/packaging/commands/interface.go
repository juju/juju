// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

// Package commands contains an interface which returns common
// package-manager related commands and the reference implementation for apt
// and yum-based systems.
package commands

import (
	"github.com/juju/utils/proxy"
)

// PackageCommander is the interface which provides runnable shell
// commands for various packaging-related operations.
type PackageCommander interface {
	// InstallPrerequisiteCmd returns the command that installs the
	// prerequisite package for repository-handling operations.
	InstallPrerequisiteCmd() string

	// UpdateCmd returns the command to update the local package list.
	UpdateCmd() string

	// UpgradeCmd returns the command which issues an upgrade on all packages
	// with available newer versions.
	UpgradeCmd() string

	// InstallCmd returns a *single* command that installs the given package(s).
	InstallCmd(...string) string

	// RemoveCmd returns a *single* command that removes the given package(s).
	RemoveCmd(...string) string

	// PurgeCmd returns the command that removes the given package(s) along
	// with any associated config files.
	PurgeCmd(...string) string

	// IsInstalledCmd returns the command which determines whether or not a
	// package is currently installed on the system.
	IsInstalledCmd(string) string

	// SearchCmd returns the command that determines whether the given package is
	// available for installation from the currently configured repositories.
	SearchCmd(string) string

	// ListAvailableCmd returns the command which will list all packages
	// available for installation from the currently configured repositories.
	// NOTE: includes already installed packages.
	ListAvailableCmd() string

	// ListInstalledCmd returns the command which will list all
	// packages currently installed on the system.
	ListInstalledCmd() string

	// ListRepositoriesCmd returns the command that lists all repositories
	// currently configured on the system.
	// NOTE: requires the prerequisite package whose installation command
	// is given by InstallPrerequisiteCmd().
	ListRepositoriesCmd() string

	// AddRepositoryCmd returns the command that adds a repository to the
	// list of available repositories.
	// NOTE: requires the prerequisite package whose installation command
	// is given by InstallPrerequisiteCmd().
	AddRepositoryCmd(string) string

	// RemoveRepositoryCmd returns the command that removes a given
	// repository from the list of available repositories.
	// NOTE: requires the prerequisite package whose installation command
	// is given by InstallPrerequisiteCmd().
	RemoveRepositoryCmd(string) string

	// CleanupCmd returns the command that cleans up all orphaned packages,
	// left-over files and previously-cached packages.
	CleanupCmd() string

	// GetProxyCmd returns the command which outputs the proxies set for the
	// given package management system.
	// NOTE: output may require some additional filtering.
	GetProxyCmd() string

	// ProxyConfigContents returns the format expected by the package manager
	// for proxy settings which can be written directly to the config file.
	ProxyConfigContents(proxy.Settings) string

	// SetProxyCmds returns the commands which write the proxy configuration
	// to the configuration file of the package manager.
	SetProxyCmds(proxy.Settings) []string
}

// NewPackageCommander returns a new PackageCommander instance based on the
// given series.
func NewPackageCommander(series string) (PackageCommander, error) {
	// TODO (aznashwan): find a more deterministic way of selection here which
	// does not imply importing version from core.
	switch series {
	case "centos7":
		return NewYumPackageCommander(), nil
	default:
		return NewAptPackageCommander(), nil
	}
	return nil, nil
}

// NewAptPackageCommander returns a PackageCommander for apt-based systems.
func NewAptPackageCommander() PackageCommander {
	return &aptCmder
}

// NewYumPackageCommander returns a PackageCommander for yum-based systems.
func NewYumPackageCommander() PackageCommander {
	return &yumCmder
}
