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
	update                string                        // updates the local package list
	upgrade               string                        // upgrades all packages
	install               string                        // installs the given packages
	addRepository         string                        // adds the given repository
	proxySettingsFormat   string                        // format for proxy setting in package manager config file
	setProxy              string                        // command for adding a proxy setting to the config file
	noProxySettingsFormat string                        // format for no-proxy setting in package manager config file
	setMirrorCommands     func(string, string) []string // updates archive and security package manager to use the given mirrors
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

// AddRepositoryCmd is defined on the PackageCommander interface.
func (p *packageCommander) AddRepositoryCmd(repo string) string {
	return fmt.Sprintf(p.addRepository, repo)
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

	addOption("http", settings.Http)
	addOption("https", settings.Https)
	addOption("ftp", settings.Ftp)

	addNoProxyCmd := func(protocol, host string) {
		options = append(options, p.giveNoProxyOption(protocol, host))
	}

	if p.noProxySettingsFormat != "" {
		for _, host := range strings.Split(settings.NoProxy, ",") {
			if host != "" {
				addNoProxyCmd("http", host)
				addNoProxyCmd("https", host)
				addNoProxyCmd("ftp", host)
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
