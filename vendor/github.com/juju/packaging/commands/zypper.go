// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.
// Created from yum.go

package commands

const (
	// OpenSUSESourcesDir is the default directory in which openSUSE sourcefiles
	// may be found.
	OpenSUSESourcesDir = "/etc/zypp/repos.d"

	// OpenSUSECredentialsDir is the sirectory for credentials.
	OpenSUSECredentialsDir = "/etc/zypp/credentials.d"

	// OpenSUSESourcesFile is the default file which lists all core sources
	// for zypper packages on OpenSUSE.
	OpenSUSESourcesFile = "/etc/zypp/repos.d/repo-oss.repo"

	// ZypperConfigFile is the default configuration file for yum settings.
	ZypperConfigFilePath = "/etc/zypp/zypp.conf"

	//OpenSUSE proxy settings
	OpenSUSEProxy = "/etc/sysconfig/proxy"
)

const (
	// Zypper command for managing packages and repos
	//		--quiet to only show errors
	//		--non-interactive to install without asking packages
	zypper = "zypper --quiet --non-interactive"

	// OpenSUSE format for proxy environment variables
	zypperProxySettingFormat = "%s_PROXY=%s"
)

// zypperCmder is the packageCommander instantiation for zypper-based systems.
var zypperCmder = packageCommander{
	prereq:              buildCommand(":", "#No action here"),
	update:              buildCommand(zypper, "refresh"),
	upgrade:             buildCommand(zypper, "update"),
	install:             buildCommand(zypper, "install"),
	remove:              buildCommand(zypper, "remove"),
	purge:               buildCommand(zypper, "remove"), // No purges with zypper
	search:              buildCommand(zypper, "search %s"),
	isInstalled:         buildCommand(zypper, "search -i %s"),
	listAvailable:       buildCommand(zypper, "packages"),
	listInstalled:       buildCommand(zypper, "packages -i"),
	listRepositories:    buildCommand(zypper, "repos"),
	addRepository:       buildCommand(zypper, "addrepo %s"),
	removeRepository:    buildCommand(zypper, "removerepo %s"),
	cleanup:             buildCommand(zypper, "clean --all"),
	getProxy:            buildCommand("grep -R \".*_PROXY=\"", OpenSUSEProxy),
	proxySettingsFormat: zypperProxySettingFormat,
	setProxy: buildCommand("sed -ie 's/PROXY_ENABLED=\"no\"/PROXY_ENABLED=\"yes\"/g'",
		OpenSUSEProxy,
		"echo %s >>",
		OpenSUSEProxy),
	setNoProxy:          buildCommand("echo %s >> ", OpenSUSEProxy),
	proxyLabelInCapital: true,
}
