// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package packaging

const (
	// UbuntuAptSourcesFile is the default file which list all core
	// sources for apt packages on an Ubuntu system.
	UbuntuAptSourcesFile = "/etc/apt/sources.list"

	// AptPreferenceTemplate is the template specific to an apt preference file.
	AptPreferenceTemplate = `
Explanation: {{.Explanation}}
Package: {{.Package}}
Pin: {{.Pin}}
Pin-Priority: {{.PinPriority}}
`

	// the basic command for all apt-get calls
	//		--assume-yes to never prompt for confirmation
	//		--force-confold is passed to dpkg to never overwrite config files
	aptget = "apt-get --assume-yes --option Dpkg::Options::=--force-confold "

	// the basic command for all apt-cache calls
	aptcache = "apt-cache "

	// the basic command for all add-apt-repository calls
	//		--yes to never prompt for confirmation
	addaptrepo = "add-apt-repository --yes "
)

// aptCmds is a map of available actions specific to a package manager
// and their direct equivalent command on an apt-based system.
var aptCmds map[string]string = map[string]string{
	"update":            aptget + "update",
	"upgrade":           aptget + "upgrade",
	"install":           aptget + "install ",
	"remove":            aptget + "remove ",
	"purge":             aptget + "purge ",
	"search":            aptcache + "search --names-only ^%s$",
	"list-available":    aptcache + "pkgnames",
	"list-installed":    "dpkg --get-selections",
	"add-repository":    addaptrepo + "ppa:%s",
	"remove-repository": addaptrepo + "--remove ppa:%s",
	"cleanup":           aptget + "autoremove",
}
