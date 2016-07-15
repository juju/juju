// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"

	"github.com/juju/utils/proxy"
)

// packageCommander is a struct which returns system-specific commands for all
// the operations that may be required of a package management system.
// It implements the PackageCommander interface.
type packageCommander struct {
	prereq              string // installs prerequisite repo management package
	update              string // updates the local package list
	upgrade             string // upgrades all packages
	install             string // installs the given packages
	remove              string // removes the given packages
	purge               string // removes the given packages along with all data
	search              string // searches for the given package
	isInstalled         string // checks if a given package is installed
	listAvailable       string // lists all packes available
	listInstalled       string // lists all installed packages
	listRepositories    string // lists all currently configured repositories
	addRepository       string // adds the given repository
	removeRepository    string // removes the given repository
	cleanup             string // cleans up orhaned packages and the package cache
	getProxy            string // command for getting the currently set packagemanager proxy
	proxySettingsFormat string // format for proxy setting in package manager config file
	setProxy            string // command for adding a proxy setting to the config file
}

// InstallPrerequisiteCmd is defined on the PackageCommander interface.
func (p *packageCommander) InstallPrerequisiteCmd() string {
	return p.prereq
}

// UpdateCmd is defined on the PackageCommander interface.
func (p *packageCommander) UpdateCmd() string {
	return p.update
}

// UpgradeCmd is defined on the PackageCommander interface.
func (p *packageCommander) UpgradeCmd() string {
	return p.upgrade
}

// InstallCmd is defined on the PackageCommander interface.
func (p *packageCommander) InstallCmd(packs ...string) string {
	return addArgsToCommand(p.install, packs)
}

// RemoveCmd is defined on the PackageCommander interface.
func (p *packageCommander) RemoveCmd(packs ...string) string {
	return addArgsToCommand(p.remove, packs)
}

// PurgeCmd is defined on the PackageCommander interface.
func (p *packageCommander) PurgeCmd(packs ...string) string {
	return addArgsToCommand(p.purge, packs)
}

// SearchCmd is defined on the PackageCommander interface.
func (p *packageCommander) SearchCmd(pack string) string {
	return fmt.Sprintf(p.search, pack)
}

// IsInstalledCmd is defined on the PackageCommander interface.
func (p *packageCommander) IsInstalledCmd(pack string) string {
	return fmt.Sprintf(p.isInstalled, pack)
}

// ListAvailableCmd is defined on the PackageCommander interface.
func (p *packageCommander) ListAvailableCmd() string {
	return p.listAvailable
}

// ListInstalledCmd is defined on the PackageCommander interface.
func (p *packageCommander) ListInstalledCmd() string {
	return p.listInstalled
}

// ListRepositoriesCmd is defined on the PackageCommander interface.
func (p *packageCommander) ListRepositoriesCmd() string {
	return p.listRepositories
}

// AddRepositoryCmd is defined on the PackageCommander interface.
func (p *packageCommander) AddRepositoryCmd(repo string) string {
	return fmt.Sprintf(p.addRepository, repo)
}

// RemoveRepositoryCmd is defined on the PackageCommander interface.
func (p *packageCommander) RemoveRepositoryCmd(repo string) string {
	return fmt.Sprintf(p.removeRepository, repo)
}

// CleanupCmd is defined on the PackageCommander interface.
func (p *packageCommander) CleanupCmd() string {
	return p.cleanup
}

// GetProxyCmd is defined on the PackageCommander interface.
func (p *packageCommander) GetProxyCmd() string {
	return p.getProxy
}

// giveProxyOptions is a helper function which takes a possible proxy setting
// and its value and returns the formatted option for it.
func (p *packageCommander) giveProxyOption(setting, proxy string) string {
	return fmt.Sprintf(p.proxySettingsFormat, setting, proxy)
}

// ProxyConfigContents is defined on the PackageCommander interface.
func (p *packageCommander) ProxyConfigContents(settings proxy.Settings) string {
	options := []string{}

	addOption := func(setting, proxy string) {
		if proxy != "" {
			options = append(options, p.giveProxyOption(setting, proxy))
		}
	}

	addOption("http", settings.Http)
	addOption("https", settings.Https)
	addOption("ftp", settings.Ftp)

	return strings.Join(options, "\n")
}

// SetProxyCmds is defined on the PackageCommander interface.
func (p *packageCommander) SetProxyCmds(settings proxy.Settings) []string {
	cmds := []string{}

	addProxyCmd := func(setting, proxy string) {
		if proxy != "" {
			cmds = append(cmds, fmt.Sprintf(p.setProxy, p.giveProxyOption(setting, proxy)))
		}
	}

	addProxyCmd("http", settings.Http)
	addProxyCmd("https", settings.Https)
	addProxyCmd("ftp", settings.Ftp)

	return cmds
}
