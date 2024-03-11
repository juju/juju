// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package paths

import (
	"os"
	"runtime"

	jujuos "github.com/juju/juju/core/os"
)

type OS int // strongly typed runtime.GOOS value to help with refactoring

const (
	OSWindows  OS = 1
	OSUnixLike OS = 2
)

type osVarType int

const (
	tmpDir osVarType = iota
	logDir
	dataDir
	storageDir
	confDir
	jujuExec
	certDir
	metricsSpoolDir
	uniterStateDir
	jujuDumpLogs
	jujuIntrospect
	instanceCloudInitDir
	cloudInitCfgDir
	curtinInstallConfig
	transientDataDir
)

const (
	// NixDataDir is location for agent binaries on *nix operating systems.
	NixDataDir = "/var/lib/juju"

	// NixTransientDataDir is location for storing transient data on *nix
	// operating systems.
	NixTransientDataDir = "/var/run/juju"

	// NixLogDir is location for Juju logs on *nix operating systems.
	NixLogDir = "/var/log"
)

var nixVals = map[osVarType]string{
	tmpDir:               "/tmp",
	logDir:               NixLogDir,
	dataDir:              NixDataDir,
	transientDataDir:     NixTransientDataDir,
	storageDir:           "/var/lib/juju/storage",
	confDir:              "/etc/juju",
	jujuExec:             "/usr/bin/juju-exec",
	jujuDumpLogs:         "/usr/bin/juju-dumplogs",
	jujuIntrospect:       "/usr/bin/juju-introspect",
	certDir:              "/etc/juju/certs.d",
	metricsSpoolDir:      "/var/lib/juju/metricspool",
	uniterStateDir:       "/var/lib/juju/uniter/state",
	instanceCloudInitDir: "/var/lib/cloud/instance",
	cloudInitCfgDir:      "/etc/cloud/cloud.cfg.d",
	curtinInstallConfig:  "/root/curtin-install-cfg.yaml",
}

var winVals = map[osVarType]string{
	tmpDir:           "C:/Juju/tmp",
	logDir:           "C:/Juju/log",
	dataDir:          "C:/Juju/lib/juju",
	transientDataDir: "C:/Juju/lib/juju-transient",
	storageDir:       "C:/Juju/lib/juju/storage",
	confDir:          "C:/Juju/etc",
	jujuExec:         "C:/Juju/bin/juju-exec.exe",
	jujuDumpLogs:     "C:/Juju/bin/juju-dumplogs.exe",
	jujuIntrospect:   "C:/Juju/bin/juju-introspect.exe",
	certDir:          "C:/Juju/certs",
	metricsSpoolDir:  "C:/Juju/lib/juju/metricspool",
	uniterStateDir:   "C:/Juju/lib/juju/uniter/state",
}

// Chown is a variable here so it can be mocked out in tests to a no-op.
// Agents run as root, but users don't.
var Chown = os.Chown

// CurrentOS returns the OS value for the currently-running system.
func CurrentOS() OS {
	switch runtime.GOOS {
	case "windows":
		return OSWindows
	default:
		return OSUnixLike
	}
}

// OSType converts the given os name to an OS value.
func OSType(osName string) OS {
	switch jujuos.OSTypeForName(osName) {
	case jujuos.Windows:
		return OSWindows
	default:
		return OSUnixLike
	}
}

// osVal will lookup the value of the key valname
// in the appropriate map, based on the OS value.
func osVal(os OS, valname osVarType) string {
	switch os {
	case OSWindows:
		return winVals[valname]
	default:
		return nixVals[valname]
	}
}

// LogDir returns filesystem path the directory where juju may
// save log files.
func LogDir(os OS) string {
	return osVal(os, logDir)
}

// DataDir returns a filesystem path to the folder used by juju to
// store tools, charms, locks, etc
func DataDir(os OS) string {
	return osVal(os, dataDir)
}

// TransientDataDir returns a filesystem path to the folder used by juju to
// store transient data that will not survive a reboot.
func TransientDataDir(os OS) string {
	return osVal(os, transientDataDir)
}

// MetricsSpoolDir returns a filesystem path to the folder used by juju
// to store metrics.
func MetricsSpoolDir(os OS) string {
	return osVal(os, metricsSpoolDir)
}

// CertDir returns a filesystem path to the folder used by juju to
// store certificates that are added by default to the Juju client
// api certificate pool.
func CertDir(os OS) string {
	return osVal(os, certDir)
}

// StorageDir returns a filesystem path to the folder used by juju to
// mount machine-level storage.
func StorageDir(os OS) string {
	return osVal(os, storageDir)
}

// ConfDir returns the path to the directory where Juju may store
// configuration files.
func ConfDir(os OS) string {
	return osVal(os, confDir)
}

// JujuExec returns the absolute path to the juju-exec binary for
// a particular series.
func JujuExec(os OS) string {
	return osVal(os, jujuExec)
}

// JujuDumpLogs returns the absolute path to the juju-dumplogs binary
// for a particular series.
func JujuDumpLogs(os OS) string {
	return osVal(os, jujuDumpLogs)
}

// JujuIntrospect returns the absolute path to the juju-introspect
// binary for a particular series.
func JujuIntrospect(os OS) string {
	return osVal(os, jujuIntrospect)
}

// MachineCloudInitDir returns the absolute path to the instance
// cloudinit directory for a particular series.
func MachineCloudInitDir(os OS) string {
	return osVal(os, instanceCloudInitDir)
}

// CurtinInstallConfig returns the absolute path the configuration file
// written by Curtin during machine provisioning.
func CurtinInstallConfig(os OS) string {
	return osVal(os, curtinInstallConfig)
}

// CloudInitCfgDir returns the absolute path to the instance
// cloud config directory for a particular series.
func CloudInitCfgDir(os OS) string {
	return osVal(os, cloudInitCfgDir)
}
