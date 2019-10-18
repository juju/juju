// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package paths

import (
	"os"
	"os/user"
	"strconv"

	"github.com/juju/errors"
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
	LogfilePermission = os.FileMode(0640)
)

const (
	// NixDataDir is location for agent binaries on *nix operating systems.
	NixDataDir = "/var/lib/juju"

	// NixDataDir is location for Juju logs on *nix operating systems.
	NixLogDir = "/var/log"
)

var nixVals = map[osVarType]string{
	tmpDir:               "/tmp",
	logDir:               NixLogDir,
	dataDir:              NixDataDir,
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

func SetOwnerGroupLog(filePath string, wantedOwner string, wantedGroup string) error {
	group, err := user.LookupGroup(wantedGroup)
	if err != nil {
		return errors.Trace(err)
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return errors.Trace(err)
	}
	usr, err := user.Lookup(wantedOwner)
	if err != nil {
		return errors.Trace(err)
	}
	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		return errors.Trace(err)
	}
	return os.Chown(filePath, uid, gid)
}

// PrimeLogFile ensures that the given log file is created with the
// correct mode and ownership.
func PrimeLogFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, LogfilePermission)
	if err != nil {
		return errors.Trace(err)
	}
	if err := f.Close(); err != nil {
		return errors.Trace(err)
	}
	wantedOwner, wantedGroup := SyslogUserGroup()
	return SetOwnerGroupLog(path, wantedOwner, wantedGroup)
}

// SyslogUserGroup returns the names of the user and group that own the log files.
func SyslogUserGroup() (string, string) {
	switch jujuos.HostOS() {
	case jujuos.CentOS:
		return "root", "adm"
	case jujuos.OpenSUSE:
		return "root", "root"
	default:
		return "syslog", "adm"
	}
}
