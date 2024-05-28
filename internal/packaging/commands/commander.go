// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"

	"github.com/juju/proxy"
)

// packageCommander is a struct which returns system-specific commands for all
// the operations that may be required of a package management system.
// It implements the PackageCommander interface.
type packageCommander struct {
	prereq                string                        // installs prerequisite repo management package
	update                string                        // updates the local package list
	upgrade               string                        // upgrades all packages
	install               string                        // installs the given packages
	remove                string                        // removes the given packages
	purge                 string                        // removes the given packages along with all data
	search                string                        // searches for the given package
	isInstalled           string                        // checks if a given package is installed
	listAvailable         string                        // lists all packes available
	listInstalled         string                        // lists all installed packages
	listRepositories      string                        // lists all currently configured repositories
	addRepository         string                        // adds the given repository
	removeRepository      string                        // removes the given repository
	cleanup               string                        // cleans up orhaned packages and the package cache
	getProxy              string                        // command for getting the currently set packagemanager proxy
	proxySettingsFormat   string                        // format for proxy setting in package manager config file
	setProxy              string                        // command for adding a proxy setting to the config file
	setNoProxy            string                        // command for adding a no-proxy setting to the config file
	noProxySettingsFormat string                        // format for no-proxy setting in package manager config file
	proxyLabelInCapital   bool                          // true: proxy labels are in capital letter (e.g. HTTP_PROXY)
	setMirrorCommands     func(string, string) []string // updates archive and security package manager to use the given mirrors
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

// giveNoProxyOptions is a helper function which takes protocol and hostname
// and returns formatted options for NoProxy setting
func (p *packageCommander) giveNoProxyOption(protocol, hostname string) string {
	return fmt.Sprintf(p.noProxySettingsFormat, protocol, hostname)
}

func (p *packageCommander) proxyConfigLines(settings proxy.Settings) []string {
	options := []string{}

	addOption := func(setting, proxy string) {
		if proxy != "" {
			options = append(options, p.giveProxyOption(setting, proxy))
		}
	}

	// OpenSUSE uses proxy labels in capital letter (e.g HTTP_PROXY)
	// For backward compatibility I included a flag in packageCommander.
	var http_label, https_label, ftp_label string
	if p.proxyLabelInCapital {
		http_label = "HTTP"
		https_label = "HTTPS"
		ftp_label = "FTP"
	} else {
		http_label = "http"
		https_label = "https"
		ftp_label = "ftp"
	}
	addOption(http_label, settings.Http)
	addOption(https_label, settings.Https)
	addOption(ftp_label, settings.Ftp)

	addNoProxyCmd := func(protocol, host string) {
		options = append(options, p.giveNoProxyOption(protocol, host))
	}

	if p.noProxySettingsFormat != "" {
		for _, host := range strings.Split(settings.NoProxy, ",") {
			if host != "" {
				addNoProxyCmd(http_label, host)
				addNoProxyCmd(https_label, host)
				addNoProxyCmd(ftp_label, host)
			}
		}
	}
	return options
}

// ProxyConfigContents is defined on the PackageCommander interface.
func (p *packageCommander) ProxyConfigContents(settings proxy.Settings) string {
	return strings.Join(p.proxyConfigLines(settings), "\n")
}

// SetProxyCmds is defined on the PackageCommander interface.
func (p *packageCommander) SetProxyCmds(settings proxy.Settings) []string {
	options := p.proxyConfigLines(settings)
	cmds := []string{}
	for _, option := range options {
		cmds = append(cmds, fmt.Sprintf(p.setProxy, option))
	}
	return cmds
}

// SetMirrorCommands is defined on the PackageCommander interface.
func (p *packageCommander) SetMirrorCommands(archiveMirror, securityMirror string) []string {
	if p.setMirrorCommands == nil {
		return nil
	}
	return p.setMirrorCommands(archiveMirror, securityMirror)
}
