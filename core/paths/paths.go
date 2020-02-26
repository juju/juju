// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package paths

import (
	"os"

	jujuos "github.com/juju/os"
	"github.com/juju/os/series"
)

type osVarType int

const (
	tmpDir osVarType = iota
	logDir
	dataDir
	storageDir
	confDir
	jujuRun
	certDir
	metricsSpoolDir
	uniterStateDir
	jujuDumpLogs
	jujuIntrospect
	jujuUpdateSeries
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
	jujuRun:              "/usr/bin/juju-run",
	jujuDumpLogs:         "/usr/bin/juju-dumplogs",
	jujuIntrospect:       "/usr/bin/juju-introspect",
	jujuUpdateSeries:     "/usr/bin/juju-updateseries",
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
	jujuRun:          "C:/Juju/bin/juju-run.exe",
	jujuDumpLogs:     "C:/Juju/bin/juju-dumplogs.exe",
	jujuIntrospect:   "C:/Juju/bin/juju-introspect.exe",
	jujuUpdateSeries: "C:/Juju/bin/juju-updateseries.exe",
	certDir:          "C:/Juju/certs",
	metricsSpoolDir:  "C:/Juju/lib/juju/metricspool",
	uniterStateDir:   "C:/Juju/lib/juju/uniter/state",
}

// Chown is a variable here so it can be mocked out in tests to a no-op.
// Agents run as root, but users don't.
var Chown = os.Chown

// osVal will lookup the value of the key valname
// in the appropriate map, based on the series. This will
// help reduce boilerplate code
func osVal(ser string, valname osVarType) (string, error) {
	os, err := series.GetOSFromSeries(ser)
	if err != nil {
		return "", err
	}
	switch os {
	case jujuos.Windows:
		return winVals[valname], nil
	default:
		return nixVals[valname], nil
	}
}

// TempDir returns the path on disk to the correct tmp directory
// for the series. This value will be the same on virtually
// all linux systems, but will differ on windows
func TempDir(series string) (string, error) {
	return osVal(series, tmpDir)
}

// LogDir returns filesystem path the directory where juju may
// save log files.
func LogDir(series string) (string, error) {
	return osVal(series, logDir)
}

// DataDir returns a filesystem path to the folder used by juju to
// store tools, charms, locks, etc
func DataDir(series string) (string, error) {
	return osVal(series, dataDir)
}

// TransientDataDir returns a filesystem path to the folder used by juju to
// store transient data that will not survive a reboot.
func TransientDataDir(series string) (string, error) {
	return osVal(series, transientDataDir)
}

// MetricsSpoolDir returns a filesystem path to the folder used by juju
// to store metrics.
func MetricsSpoolDir(series string) (string, error) {
	return osVal(series, metricsSpoolDir)
}

// CertDir returns a filesystem path to the folder used by juju to
// store certificates that are added by default to the Juju client
// api certificate pool.
func CertDir(series string) (string, error) {
	return osVal(series, certDir)
}

// StorageDir returns a filesystem path to the folder used by juju to
// mount machine-level storage.
func StorageDir(series string) (string, error) {
	return osVal(series, storageDir)
}

// ConfDir returns the path to the directory where Juju may store
// configuration files.
func ConfDir(series string) (string, error) {
	return osVal(series, confDir)
}

// JujuRun returns the absolute path to the juju-run binary for
// a particular series.
func JujuRun(series string) (string, error) {
	return osVal(series, jujuRun)
}

// JujuDumpLogs returns the absolute path to the juju-dumplogs binary
// for a particular series.
func JujuDumpLogs(series string) (string, error) {
	return osVal(series, jujuDumpLogs)
}

// JujuIntrospect returns the absolute path to the juju-introspect
// binary for a particular series.
func JujuIntrospect(series string) (string, error) {
	return osVal(series, jujuIntrospect)
}

// MachineCloudInitDir returns the absolute path to the instance
// cloudinit directory for a particular series.
func MachineCloudInitDir(series string) (string, error) {
	return osVal(series, instanceCloudInitDir)
}

// CurtinInstallConfig returns the absolute path the configuration file
// written by Curtin during machine provisioning.
func CurtinInstallConfig(series string) (string, error) {
	return osVal(series, curtinInstallConfig)
}

// CloudInitCfgDir returns the absolute path to the instance
// cloud config directory for a particular series.
func CloudInitCfgDir(series string) (string, error) {
	return osVal(series, cloudInitCfgDir)
}

// JujuUpdateSeries returns the absolute path to the juju-updateseries
// binary for a particular series.
func JujuUpdateSeries(series string) (string, error) {
	return osVal(series, jujuUpdateSeries)
}

func MustSucceed(s string, e error) string {
	if e != nil {
		panic(e)
	}
	return s
}
