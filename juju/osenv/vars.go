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

type OsVars struct {
	Temp       string
	Lib        string
	Log        string
	Data       string
	JujuRun    string
	SocketType string
	MustReboot string
}

// Paths speciffic to Windows
func WinEnv() OsVars {
	return OsVars{
		Temp:       "C:/Juju/tmp",
		Lib:        "C:/Juju/lib",
		Log:        "C:/Juju/log",
		Data:       "C:/Juju/lib/juju",
		JujuRun:    "C:/Juju/bin/juju-run",
		SocketType: "tcp",
		MustReboot: "1001",
	}
}

// Paths speciffic to Ubuntu
func UbuntuEnv() OsVars {
	return OsVars{
		Temp:       "/tmp",
		Lib:        "/var/lib",
		Log:        "/var/log",
		Data:       "/var/lib/juju",
		JujuRun:    "/usr/local/bin/juju-run",
		SocketType: "unix",
		MustReboot: "101",
	}
}

// gsamfira: Temporary function to return correct
// variables. This will need to become an actual function
// suitable for returning the system we are running on
func OsVersion() string {
	if runtime.GOOS == "windows" {
		return "windows"
	}
	return "ubuntu"
}

func NewOsVars(osystem string) OsVars {
	imap := map[string]interface{}{
		"ubuntu":  UbuntuEnv,
		"windows": WinEnv,
	}
	return imap[osystem].(func() OsVars)()
}
