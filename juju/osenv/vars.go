// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"fmt"

	"github.com/juju/juju/version"
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

var linuxVals = map[string]string{
	"TmpDir":  "/tmp",
	"LogDir":  "/var/log",
	"DataDir": "/var/lib/juju",
	"Jujurun": "/usr/local/bin/juju-run",
}

var winVals = map[string]string{
	"TmpDir":  "C:/Juju/tmp",
	"LogDir":  "C:/Juju/log",
	"DataDir": "C:/Juju/lib/juju",
	"JujuRun": "C:/Juju/bin/juju-run",
}

// osVal will lookup the value of the key valname
// in the apropriate map, based on the series. This will
// help reduce boilerplate code
func osVal(series, valname string) (string, error) {
	os, err := version.GetOSFromSeries(series)
	if err != nil {
		return "", err
	}
	switch os {
	case "windows":
		return winVals[valname], nil
	case "ubuntu":
		return linuxVals[valname], nil
	}
	return "", fmt.Errorf("Unknown OS: %q", os)
}

// TempDir returns the path on disk to the corect tmp directory
// for the series. This value will be the same on virtually
// all linux systems, but will differ on windows
func TempDir(series string) (string, error) {
	return osVal(series, "TmpDir")
}

// LogDir returns filesystem path the directory where juju may
// save log files.
func LogDir(series string) (string, error) {
	return osVal(series, "LogDir")
}

// DataDir returns a filesystem path to the folder used by juju to
// store tools, charms, locks, etc
func DataDir(series string) (string, error) {
	return osVal(series, "DataDir")
}

// JujuRun returns the absolute path to the juju-run binary for
// a particula series
func JujuRun(series string) (string, error) {
	return osVal(series, "JujuRun")
}

func MustSucceed(s string, e error) string {
	if e != nil {
		panic(e)
	}
	return s
}
