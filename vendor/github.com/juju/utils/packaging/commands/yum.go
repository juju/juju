// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package commands

const (
	// CentOSSourcesDir is the default directory in which yum sourcefiles
	// may be found.
	CentOSSourcesDir = "/etc/yum/repos.d"

	// CentOSYumKeyfileDir is the default directory for yum repository keys.
	CentOSYumKeyfileDir = "/etc/pki/rpm-gpg/"

	// CentOSSourcesFile is the default file which lists all core sources
	// for yum packages on CentOS.
	CentOSSourcesFile = "/etc/yum/repos.d/CentOS-Base.repo"

	// YumConfigFile is the default configuration file for yum settings.
	YumConfigFilePath = "/etc/yum.conf"
)

const (
	// the basic command for all yum calls
	// 		--assumeyes to never prompt for confirmation
	//		--debuglevel=1 to limit output verbosity
	yum = "yum --assumeyes --debuglevel=1"

	// the basic command for all yum repository configuration operations.
	yumconf = "yum-config-manager"

	// the basic format for specifying a proxy setting for yum.
	// NOTE: only http(s) proxies are relevant.
	yumProxySettingFormat = "%s_proxy=%s"
)

// yumCmder is the packageCommander instantiation for yum-based systems.
var yumCmder = packageCommander{
	prereq:              buildCommand(yum, "install yum-utils"),
	update:              buildCommand(yum, "clean expire-cache"),
	upgrade:             buildCommand(yum, "update"),
	install:             buildCommand(yum, "install"),
	remove:              buildCommand(yum, "remove"),
	purge:               buildCommand(yum, "remove"), // purges by default
	search:              buildCommand(yum, "list %s"),
	isInstalled:         buildCommand(yum, "list installed %s"),
	listAvailable:       buildCommand(yum, "list all"),
	listInstalled:       buildCommand(yum, "list installed"),
	listRepositories:    buildCommand(yum, "repolist all"),
	addRepository:       buildCommand(yumconf, "--add-repo %s"),
	removeRepository:    buildCommand(yumconf, "--disable %s"),
	cleanup:             buildCommand(yum, "clean all"),
	getProxy:            buildCommand("grep -R \".*_proxy=\"", YumConfigFilePath),
	proxySettingsFormat: yumProxySettingFormat,
	setProxy:            buildCommand("echo %s >>", YumConfigFilePath),
}
