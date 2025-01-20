// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/proxy"
)

// PackageCommander is the interface which provides runnable shell
// commands for various packaging-related operations.
type PackageCommander interface {
	// UpdateCmd returns the command to update the local package list.
	UpdateCmd() string

	// UpgradeCmd returns the command which issues an upgrade on all packages
	// with available newer versions.
	UpgradeCmd() string

	// InstallCmd returns a *single* command that installs the given package(s).
	InstallCmd(...string) string

	// AddRepositoryCmd returns the command that adds a repository to the
	// list of available repositories.
	AddRepositoryCmd(string) string

	// SetMirrorCommands returns the commands to update the package archive and security mirrors.
	SetMirrorCommands(string, string) []string

	// ProxyConfigContents returns the format expected by the package manager
	// for proxy settings which can be written directly to the config file.
	ProxyConfigContents(proxy.Settings) string

	// SetProxyCmds returns the commands which write the proxy configuration
	// to the configuration file of the package manager.
	SetProxyCmds(proxy.Settings) []string
}

// NewAptPackageCommander returns a PackageCommander for apt-based systems.
func NewAptPackageCommander() PackageCommander {
	return &aptCmder
}

// NewSnapPackageCommander returns a PackageCommander for snap-based systems.
func NewSnapPackageCommander() PackageCommander {
	return &snapCmder
}
