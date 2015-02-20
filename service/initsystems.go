// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/service/initsystems"
	"github.com/juju/juju/service/initsystems/upstart"
	"github.com/juju/juju/service/initsystems/windows"
	"github.com/juju/juju/version"
)

// These are the names of the juju-compatible init systems.
const (
	InitSystemWindows = windows.Name
	InitSystemUpstart = upstart.Name
)

// DiscoverInitSystem determines the name of the init system to use
// and returns it. The name is derived from the executable of PID 1.
// If that does work then the information in version.Current is used
// to decide which init system. If the init system cannot be
// discovered at all then the empty string is returned.
func DiscoverInitSystem() string {
	// First try to "read" the name.
	name := initsystems.DiscoverInitSystem()
	if name != "" {
		return name
	}

	// Fall back to checking what juju knows about the OS.
	return osInitSystem(version.Current)
}

func osInitSystem(vers version.Binary) string {
	switch vers.OS {
	case version.Windows:
		return InitSystemWindows
	case version.Ubuntu:
		switch vers.Series {
		case "precise", "quantal", "raring", "saucy", "trusty", "utopic":
			return InitSystemUpstart
		default:
			// vivid and later...
			return ""
			//return InitSystemSystemd
		}
	case version.CentOS:
		return ""
		//return InitSystemSystemd
	default:
		return ""
	}
}
