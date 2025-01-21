// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"

	"github.com/juju/proxy"

	"github.com/juju/juju/internal/packaging/config"
)

const (
	// AptConfFilePath is the full file path for the proxy settings that are
	// written by cloud-init and the machine environ worker.
	AptConfFilePath = "/etc/apt/apt.conf.d/95-juju-proxy-settings"

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

	// the basic format for specifying a proxy option for apt:
	aptProxySettingFormat = "Acquire::%s::Proxy %q;"

	// disable proxy for a specific host
	aptNoProxySettingFormat = "Acquire::%s::Proxy::%q \"DIRECT\";"
)

var (
	// listRepositoriesCmd is a shell command that will list all the currently
	// configured apt repositories.
	listRepositoriesCmd = buildCommand(aptcache, `policy | grep http | awk '{ $1="" ; print }' | sed 's/^ //g'`)

	// extractAptArchiveSource is a shell command that will extract the
	// currently configured APT archive source location. We assume that
	// the first source for "main" in the file is the one that
	// should be replaced throughout the file.
	extractAptArchiveSource = buildCommand(listRepositoriesCmd, ` | grep "$(lsb_release -c -s)/main" | awk '{print $1; exit}'`)

	// extractAptSecuritySource is a shell command that will extract the
	// currently configured APT security source location. We assume that
	// the first source for "main" in the file is the one that
	// should be replaced throughout the file.
	extractAptSecuritySource = buildCommand(listRepositoriesCmd, ` | grep "$(lsb_release -c -s)-security/main" | awk '{print $1; exit}'`)
)

// NewAptPackageCommander returns an AptPackageCommander.
func NewAptPackageCommander() AptPackageCommander {
	return AptPackageCommander{}
}

// AptPackageCommander provides runnable shell commands for various apt-based
// packaging operations.
type AptPackageCommander struct{}

// UpdateCmd returns the command to update the local package list.
func (AptPackageCommander) UpdateCmd() string {
	return buildCommand(aptget, "update")
}

// UpgradeCmd returns the command which issues an upgrade on all packages
// with available newer versions.
func (AptPackageCommander) UpgradeCmd() string {
	return buildCommand(aptget, "upgrade")
}

// InstallCmd returns a *single* command that installs the given package(s).
func (AptPackageCommander) InstallCmd(packages ...string) string {
	args := append([]string{aptget, "install"}, packages...)
	return buildCommand(args...)
}

// AddRepositoryCmd returns the command that adds a repository to the
// list of available repositories.
func (AptPackageCommander) AddRepositoryCmd(repo string) string {
	return buildCommand(addaptrepo, fmt.Sprintf("%q", repo))
}

// SetMirrorCommands returns the commands to update the package archive and security mirrors.
func (AptPackageCommander) SetMirrorCommands(newArchiveMirror, newSecurityMirror string) []string {
	var cmds []string
	if newArchiveMirror != "" {
		cmds = append(cmds, fmt.Sprintf("old_archive_mirror=$(%s)", extractAptArchiveSource))
		cmds = append(cmds, fmt.Sprintf("new_archive_mirror=%q", newArchiveMirror))
		cmds = append(cmds, fmt.Sprintf("[ -f %q ] && sed -i s,$old_archive_mirror,$new_archive_mirror, %q", config.LegacyAptSourcesFile, config.LegacyAptSourcesFile))
		cmds = append(cmds, fmt.Sprintf("[ -f %q ] && sed -i s,$old_archive_mirror,$new_archive_mirror, %q", config.AptSourcesFile, config.AptSourcesFile))
		cmds = append(cmds, renameAptListFilesCommands("$new_archive_mirror", "$old_archive_mirror")...)
	}
	if newSecurityMirror != "" {
		cmds = append(cmds, fmt.Sprintf("old_security_mirror=$(%s)", extractAptSecuritySource))
		cmds = append(cmds, fmt.Sprintf("new_security_mirror=%q", newSecurityMirror))
		cmds = append(cmds, fmt.Sprintf("[ -f %q ] && sed -i s,$old_security_mirror,$new_security_mirror, %q", config.LegacyAptSourcesFile, config.LegacyAptSourcesFile))
		cmds = append(cmds, fmt.Sprintf("[ -f %q ] && sed -i s,$old_security_mirror,$new_security_mirror, %q", config.AptSourcesFile, config.AptSourcesFile))
		cmds = append(cmds, renameAptListFilesCommands("$new_security_mirror", "$old_security_mirror")...)
	}
	return cmds
}

// ProxyConfigContents returns the format expected by the package manager
// for proxy settings which can be written directly to the config file.
func (AptPackageCommander) ProxyConfigContents(settings proxy.Settings) string {
	var lines []string
	if settings.Http != "" {
		lines = append(lines, fmt.Sprintf(aptProxySettingFormat, "http", settings.Http))
	}
	if settings.Https != "" {
		lines = append(lines, fmt.Sprintf(aptProxySettingFormat, "https", settings.Https))
	}
	if settings.Ftp != "" {
		lines = append(lines, fmt.Sprintf(aptProxySettingFormat, "ftp", settings.Ftp))
	}
	for _, host := range strings.Split(settings.NoProxy, ",") {
		if host != "" {
			lines = append(lines, fmt.Sprintf(aptNoProxySettingFormat, "http", host))
			lines = append(lines, fmt.Sprintf(aptNoProxySettingFormat, "https", host))
			lines = append(lines, fmt.Sprintf(aptNoProxySettingFormat, "ftp", host))
		}
	}
	return strings.Join(lines, "\n")
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
