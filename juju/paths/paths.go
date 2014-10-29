package paths

import (
	"fmt"

	"github.com/juju/juju/version"
)

type osVarType int

const (
	tmpDir osVarType = iota
	logDir
	dataDir
	jujuRun
)

var nixVals = map[osVarType]string{
	tmpDir:  "/tmp",
	logDir:  "/var/log",
	dataDir: "/var/lib/juju",
	jujuRun: "/usr/local/bin/juju-run",
}

var winVals = map[osVarType]string{
	tmpDir:  "C:/Juju/tmp",
	logDir:  "C:/Juju/log",
	dataDir: "C:/Juju/lib/juju",
	jujuRun: "C:/Juju/bin/juju-run.exe",
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
	default:
		return nixVals[valname], nil
	}
	return "", fmt.Errorf("Unknown OS: %q", os)
}

// TempDir returns the path on disk to the corect tmp directory
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
