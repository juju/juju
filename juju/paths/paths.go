package paths

import (
	"fmt"
	"path/filepath"

	"github.com/juju/juju/version"
)

type osVarType int

const (
	tmpDir osVarType = iota
	logDir
	dataDir
	jujuRun
)

var linuxVals = map[osVarType]string{
	tmpDir:  "/tmp",
	logDir:  "/var/log",
	dataDir: "/var/lib/juju",
	jujuRun: "/usr/local/bin/juju-run",
}

var winVals = map[osVarType]string{
	tmpDir:  "C:/Juju/tmp",
	logDir:  "C:/Juju/log",
	dataDir: "C:/Juju/lib/juju",
	jujuRun: "C:/Juju/bin/juju-run",
}

// osVal will lookup the value of the key valname
// in the apropriate map, based on the series. This will
// help reduce boilerplate code
func osVal(series string, valname osVarType) (string, error) {
	os, err := version.GetOSFromSeries(series)
	if err != nil {
		return "", err
	}
	switch os {
	case version.Windows:
		return winVals[valname], nil
	case version.Ubuntu:
		return linuxVals[valname], nil
	}
	return "", fmt.Errorf("Unknown OS: %q", os)
}

// TempDir returns the path on disk to the corect tmp directory
// for the series. This value will be the same on virtually
// all linux systems, but will differ on windows
func TempDir(series string) (string, error) {
	return osVal(series, tmpDir)
}

// NewDefaultBaseLogDir returns a filesystem path to the location
// where applications may create a folder containing logs
var NewDefaultBaseLogDir = newDefaultBaseLogDir

func newDefaultBaseLogDir() string {
	return MustSucceed(LogDir(version.Current.Series))
}

// NewDefaultLogDir will call LogDir ensuring that it succeeds
// or panicking, this is a convenience function to avoid
// DefaultLogDir to be defined directly here which causes juju
// client to panic if called on an unknown windows version.
var NewDefaultLogDir = newDefaultLogDir

func newDefaultLogDir() string {
	logDir := NewDefaultBaseLogDir()
	return filepath.Join(logDir, "juju")
}

// LogDir returns filesystem path the directory where juju may
// save log files.
func LogDir(series string) (string, error) {
	return osVal(series, logDir)
}

// NewDefaultDataDir will call DataDir ensuring that it succeeds
// or panicking. This is a convenience function to avoid
// DefaultDataDir to be defined directly here which causes juju
// client to panic if called on an unknown windows version.
var NewDefaultDataDir = newDefaultDataDir

func newDefaultDataDir() string {
	return MustSucceed(DataDir(version.Current.Series))
}

// DataDir returns a filesystem path to the folder used by juju to
// store tools, charms, locks, etc
func DataDir(series string) (string, error) {
	return osVal(series, dataDir)
}

// JujuRun returns the absolute path to the juju-run binary for
// a particula series
func JujuRun(series string) (string, error) {
	return osVal(series, jujuRun)
}

func MustSucceed(s string, e error) string {
	if e != nil {
		panic(e)
	}
	return s
}
