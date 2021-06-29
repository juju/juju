// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"github.com/juju/packaging/v2/commands"
	"github.com/juju/proxy"
)

//Implementation of PackageHelper for OpenSUSE
type openSUSEHelper struct {
	paccmder commands.PackageCommander
}

//Returns the list of required packages in OpenSUSE
func (helper openSUSEHelper) getRequiredPackages() []string {
	return []string{
		"curl",
		"bridge-utils",
		//"cloud-utils", Put as a requirement to the cloud image (requires subscription)
		"ncat",
		"tmux",
	}
}

// addPackageProxyCmd is a helper method which returns the corresponding runcmd
// to apply the package proxy settings for OpenSUSE
func (helper openSUSEHelper) addPackageProxyCmd(url string) string {
	return helper.paccmder.SetProxyCmds(proxy.Settings{
		Http: url,
	})[0]
}
