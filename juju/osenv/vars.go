// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"runtime"
)

const (
	JujuEnvEnvKey           = "JUJU_ENV"
	JujuHomeEnvKey          = "JUJU_HOME"
	JujuRepositoryEnvKey    = "JUJU_REPOSITORY"
	JujuLoggingConfigEnvKey = "JUJU_LOGGING_CONFIG"
	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	JujuContainerTypeEnvKey = "JUJU_CONTAINER_TYPE"
)

// get variables speciffic for this running system
var osystem = OsVersion()
var Vars = NewOsVars(osystem)

type OSVars struct {
	// TempDir is the path to the systems temporary folder
	TempDir string
	// LogDir is the location on disk where juju may create
	// a folder containing its logs
	LogDir string
	// DataDir is the location on disk where Juju will store its
	// tools and agent data
	DataDir string
	// JujuRun is the full path to the juju-run binary on disk
	JujuRun string
}

// WinEnv returns a OSVars instance with apropriate information
// for a Windows juju agent
func WinEnv() OSVars {
	return OSVars{
		TempDir: "C:/Juju/tmp",
		LogDir:  "C:/Juju/log",
		DataDir: "C:/Juju/lib/juju",
		JujuRun: "C:/Juju/bin/juju-run",
	}
}

// UbuntuEnv returns a OSVars instance with apropriate information
// for a Ubuntu juju agent
func UbuntuEnv() OSVars {
	return OSVars{
		TempDir: "/tmp",
		LogDir:  "/var/log",
		DataDir: "/var/lib/juju",
		JujuRun: "/usr/local/bin/juju-run",
	}
}

// OSVersion is a temporary function to return correct
// variables. This will need to become an actual function
// suitable for returning the system we are running on
func OSVersion() string {
	if runtime.GOOS == "windows" {
		return "windows"
	}
	return "ubuntu"
}

func NewOsVars(osystem string) OsVars {
	imap := map[string]func OSVars{
		"ubuntu":  UbuntuEnv,
		"windows": WinEnv,
	}
	return imap[osystem]()
}
