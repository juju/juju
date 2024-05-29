// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package commands

import "github.com/juju/juju/internal/packaging/config"

const (
	// AptConfFilePath is the full file path for the proxy settings that are
	// written by cloud-init and the machine environ worker.
	AptConfFilePath = "/etc/apt/apt.conf.d/95-juju-proxy-settings"

	// the basic command for all dpkg calls:
	dpkg = "dpkg"

	// the basic command for all dpkg-query calls:
	dpkgquery = "dpkg-query"

	// the basic command for all apt-get calls:
	//		--force-confold is passed to dpkg to never overwrite config files
	//		--force-unsafe-io makes dpkg less sync-happy
	//		--assume-yes to never prompt for confirmation
	aptget = "apt-get --option=Dpkg::Options::=--force-confold --option=Dpkg::Options::=--force-unsafe-io --assume-yes --quiet"

	// the basic command for all apt-cache calls:
	aptcache = "apt-cache"

	// the basic command for all add-apt-repository calls:
	//		--yes to never prompt for confirmation
	addaptrepo = "add-apt-repository --yes"

	// the basic command for all apt-config calls:
	aptconfig = "apt-config dump"

	// the basic format for specifying a proxy option for apt:
	aptProxySettingFormat = "Acquire::%s::Proxy %q;"

	// disable proxy for a specific host
	aptNoProxySettingFormat = "Acquire::%s::Proxy::%q \"DIRECT\";"
)

// aptCmder is the packageCommander instantiation for apt-based systems.
var aptCmder = packageCommander{
	prereq:                buildCommand(aptget, "install python-software-properties"),
	update:                buildCommand(aptget, "update"),
	upgrade:               buildCommand(aptget, "upgrade"),
	install:               buildCommand(aptget, "install"),
	remove:                buildCommand(aptget, "remove"),
	purge:                 buildCommand(aptget, "purge"),
	search:                buildCommand(aptcache, "search --names-only ^%s$"),
	isInstalled:           buildCommand(dpkgquery, "-s %s"),
	listAvailable:         buildCommand(aptcache, "pkgnames"),
	listInstalled:         buildCommand(dpkg, "--get-selections"),
	addRepository:         buildCommand(addaptrepo, "%q"),
	listRepositories:      buildCommand(`sed -r -n "s|^deb(-src)? (.*)|\2|p"`, "/etc/apt/sources.list"),
	removeRepository:      buildCommand(addaptrepo, "--remove ppa:%s"),
	cleanup:               buildCommand(aptget, "autoremove"),
	getProxy:              buildCommand(aptconfig, "Acquire::http::Proxy Acquire::https::Proxy Acquire::ftp::Proxy"),
	proxySettingsFormat:   aptProxySettingFormat,
	setProxy:              buildCommand("echo %s >> ", AptConfFilePath),
	noProxySettingsFormat: aptNoProxySettingFormat,
	setNoProxy:            buildCommand("echo %s >> ", AptConfFilePath),
	setMirrorCommands: func(newArchiveMirror, newSecurityMirror string) []string {
		var cmds []string
		if newArchiveMirror != "" {
			cmds = append(cmds, "old_archive_mirror=$("+config.ExtractAptArchiveSource+")")
			cmds = append(cmds, "new_archive_mirror="+newArchiveMirror)
			cmds = append(cmds, `sed -i s,$old_archive_mirror,$new_archive_mirror, `+config.AptSourcesFile)
			cmds = append(cmds, renameAptListFilesCommands("$new_archive_mirror", "$old_archive_mirror")...)
		}
		if newSecurityMirror != "" {
			cmds = append(cmds, "old_security_mirror=$("+config.ExtractAptSecuritySource+")")
			cmds = append(cmds, "new_security_mirror="+newSecurityMirror)
			cmds = append(cmds, `sed -i s,$old_security_mirror,$new_security_mirror, `+config.AptSourcesFile)
			cmds = append(cmds, renameAptListFilesCommands("$new_security_mirror", "$old_security_mirror")...)
		}
		return cmds
	},
}

// renameAptListFilesCommands takes a new and old mirror string,
// and returns a sequence of commands that will rename the files
// in AptListsDirectory.
func renameAptListFilesCommands(newMirror, oldMirror string) []string {
	oldPrefix := "old_prefix=" + config.AptListsDirectory + "/$(echo " + oldMirror + " | " + config.AptSourceListPrefix + ")"
	newPrefix := "new_prefix=" + config.AptListsDirectory + "/$(echo " + newMirror + " | " + config.AptSourceListPrefix + ")"
	renameFiles := `
for old in ${old_prefix}_*; do
    new=$(echo $old | sed s,^$old_prefix,$new_prefix,)
    if [ -f $old ]; then
      mv $old $new
    fi
done`

	return []string{
		oldPrefix,
		newPrefix,
		// Don't do anything unless the mirror/source has changed.
		`[ "$old_prefix" != "$new_prefix" ] &&` + renameFiles,
	}
}
