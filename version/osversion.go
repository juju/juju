// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"fmt"
	"io/ioutil"
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

func readOSRelease() (map[string]string, error) {
	values := map[string]string{}

	contents, err := ioutil.ReadFile(osReleaseFile)
	if err != nil {
		return values, err
	}
	releaseDetails := strings.Split(string(contents), "\n")
	for _, val := range releaseDetails {
		c := strings.SplitN(val, "=", 2)
		if len(c) != 2 {
			continue
		}
		values[c[0]] = strings.Trim(c[1], "\t '\"")
	}
	id, ok := values["ID"]
	if !ok {
		return values, errors.New("OS release file is missing ID")
	}
	if _, ok := values["VERSION_ID"]; !ok {
		values["VERSION_ID"], ok = defaultVersionIDs[id]
		if !ok {
			return values, errors.New("OS release file is missing VERSION_ID")
		}
	}
	return values, nil
}

func getValue(from map[string]string, val string) (string, error) {
	for serie, ver := range from {
		if ver == val {
			return serie, nil
		}
	}
	return "unknown", errors.New("Could not determine series")
}

func readSeries() (string, error) {
	values, err := readOSRelease()
	if err != nil {
		return "unknown", err
	}
	updateSeriesVersions()
	switch values["ID"] {
	case strings.ToLower(Ubuntu.String()):
		return getValue(ubuntuSeries, values["VERSION_ID"])
	case strings.ToLower(Arch.String()):
		return getValue(archSeries, values["VERSION_ID"])
	case strings.ToLower(CentOS.String()):
		codename := fmt.Sprintf("%s%s", values["ID"], values["VERSION_ID"])
		return getValue(centosSeries, codename)
	default:
		return "unknown", nil
	}
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
