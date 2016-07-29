// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// series provides helpers for determining the series of
// a host, and translating from os to series.
package series

import (
	"strconv"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/os"
)

var logger = loggo.GetLogger("juju.juju.series")

var HostSeries = hostSeries

var (
	seriesOnce sync.Once
	series     string // filled in by the first call to hostSeries
)

func hostSeries() string {
	seriesOnce.Do(func() {
		var err error
		series, err = readSeries()
		if err != nil {
			panic("unable to determine host series: " + err.Error())
		}
	})
	return series
}

// MustOSFromSeries will panic if the series represents an "unknown"
// operating system
func MustOSFromSeries(series string) os.OSType {
	operatingSystem, err := GetOSFromSeries(series)
	if err != nil {
		panic("osVersion reported an error: " + err.Error())
	}
	return operatingSystem
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
// that we want to populate HostSeries during init() time, before
// we've potentially read that information from anywhere else
// macOSXSeries maps from the Darwin Kernel Major Version to the Mac OSX
// series.
var macOSXSeries = map[int]string{
	15: "elcapitan",
	14: "yosemite",
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
