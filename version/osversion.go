// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.version")

// mustOSVersion will panic if the osVersion is "unknown" due
// to an error.
//
// If you want to avoid the panic, call osVersion and handle
// the error.
func mustOSVersion() string {
	version, err := osVersion()
	if err != nil {
		panic("osVersion reported an error: " + err.Error())
	}
	return version
}

// MustOSFromSeries will panic if the series represents an "unknown"
// operating system
func MustOSFromSeries(series string) OSType {
	operatingSystem, err := GetOSFromSeries(series)
	if err != nil {
		panic("osVersion reported an error: " + err.Error())
	}
	return operatingSystem
}

func readSeries(releaseFile string) (string, error) {
	f, err := os.Open(releaseFile)
	if err != nil {
		return "unknown", err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		const prefix = "DISTRIB_CODENAME="
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(line[len(prefix):], "\t '\""), s.Err()
		}
	}
	return "unknown", s.Err()
}

// kernelToMajor takes a dotted version and returns just the Major portion
func kernelToMajor(getKernelVersion func() (string, error)) (int, error) {
	fullVersion, err := getKernelVersion()
	if err != nil {
		return 0, err
	}
	parts := strings.SplitN(fullVersion, ".", 2)
	majorVersion, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return 0, err
	}
	return int(majorVersion), nil
}

func macOSXSeriesFromKernelVersion(getKernelVersion func() (string, error)) (string, error) {
	majorVersion, err := kernelToMajor(getKernelVersion)
	if err != nil {
		logger.Infof("unable to determine OS version: %v", err)
		return "unknown", err
	}
	return macOSXSeriesFromMajorVersion(majorVersion)
}

// TODO(jam): 2014-05-06 https://launchpad.net/bugs/1316593
// we should have a system file that we can read so this can be updated without
// recompiling Juju. For now, this is a lot easier, and also solves the fact
// that we want to populate version.Current.Series during init() time, before
// we've potentially read that information from anywhere else
// macOSXSeries maps from the Darwin Kernel Major Version to the Mac OSX
// series.
var macOSXSeries = map[int]string{
	13: "mavericks",
	12: "mountainlion",
	11: "lion",
	10: "snowleopard",
	9:  "leopard",
	8:  "tiger",
	7:  "panther",
	6:  "jaguar",
	5:  "puma",
}

func macOSXSeriesFromMajorVersion(majorVersion int) (string, error) {
	series, ok := macOSXSeries[majorVersion]
	if !ok {
		return "unknown", errors.Errorf("unknown series %q", series)
	}
	return series, nil
}
