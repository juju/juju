// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package packaging

const (
	// the basic command for all yum calls
	// 		--assumeyes to never prompt for confirmation
	//		--debuglevel=1 to limit output verbosity
	yum = "yum --assumeyes --debuglevel=1 "

	// the basic command for all yum repository configuration operations
	yumconf = "yum-config-manager "
)

// yumCmds is a map of available actions specific to a package manager
// and their direct equivalent command on a yum-based system
var yumCmds map[string]string = map[string]string{
	"update":            yum + "clean expire-cache",
	"upgrade":           yum + "update",
	"install":           yum + "install ",
	"remove":            yum + "remove ",
	"purge":             yum + "remove ", // purges by default
	"search":            yum + "list %s",
	"list-available":    yum + "list all",
	"list-installed":    yum + "list installed",
	"list-repositories": yum + "repolist all",
	"add-repository":    yumconf + "--add-repo %s",
	"remove-repository": yumconf + "--disable %s",
	"cleanup":           yum + "clean all",
}
