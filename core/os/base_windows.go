// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/errors"
)

var (
	// currentVersionKey is defined as a variable instead of a constant
	// to allow overwriting during testing
	currentVersionKey = "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion"

	// isNanoKey determines the registry key that can be queried to determine whether
	// a machine is a nano machine
	isNanoKey = "Software\\Microsoft\\Windows NT\\CurrentVersion\\Server\\ServerLevels"

	// Windows versions come in various flavors:
	// Standard, Datacenter, etc. We use string prefix match them to one
	// of the following. Specify the longest name in a particular series first
	// For example, if we have "Win 2012" and "Win 2012 R2", we specify "Win 2012 R2" first.
	// We need to make sure we manually update this list with each new windows release.
	windowsVersionMatchOrder = []string{
		"Hyper-V Server 2012 R2",
		"Hyper-V Server 2012",
		"Windows Server 2008 R2",
		"Windows Server 2012 R2",
		"Windows Server 2012",
		"Hyper-V Server 2016",
		"Windows Server 2016",
		"Windows Server 2019",
		"Windows Storage Server 2012 R2",
		"Windows Storage Server 2012",
		"Windows Storage Server 2016",
		"Windows 7",
		"Windows 8.1",
		"Windows 8",
		"Windows 10",
		"Windows 11",
	}

	// windowsVersions is a mapping consisting of the output from
	// the following WMI query: (gwmi Win32_OperatingSystem).Name
	windowsVersions = map[string]string{
		"Hyper-V Server 2012 R2":         "2012hvr2",
		"Hyper-V Server 2012":            "2012hv",
		"Windows Server 2008 R2":         "2008r2",
		"Windows Server 2012 R2":         "2012r2",
		"Windows Server 2012":            "2012",
		"Hyper-V Server 2016":            "2016hv",
		"Windows Server 2016":            "2016",
		"Windows Server 2019":            "2019",
		"Windows Storage Server 2012 R2": "2012r2",
		"Windows Storage Server 2012":    "2012",
		"Windows Storage Server 2016":    "2016",
		"Windows Storage Server 2019":    "2019",
		"Windows 7":                      "7",
		"Windows 8.1":                    "81",
		"Windows 8":                      "8",
		"Windows 10":                     "10",
		"Windows 11":                     "11",
	}

	// windowsNanoVersions is a mapping from the product name
	// stored in registry to a juju defined nano-series
	// On the nano version so far the product name actually
	// is identical to the correspondent main windows version
	// and the information about it being nano is stored in
	// a different place.
	windowsNanoVersions = map[string]string{
		"Windows Server 2016": "2016nano",
	}
)

func getVersionFromRegistry() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, currentVersionKey, registry.QUERY_VALUE)
	if err != nil {
		return "", errors.Capture(err)
	}
	defer k.Close()
	s, _, err := k.GetStringValue("ProductName")
	if err != nil {
		return "", errors.Capture(err)
	}

	return s, nil
}

func readBase() (corebase.Base, error) {
	ver, err := getVersionFromRegistry()
	if err != nil {
		return corebase.Base{}, errors.Capture(err)
	}

	var lookAt = windowsVersions

	isNano, err := isWindowsNano()
	if err != nil && os.IsNotExist(err) {
		return corebase.Base{}, errors.Capture(err)
	}
	if isNano {
		lookAt = windowsNanoVersions
	}

	for _, value := range windowsVersionMatchOrder {
		if strings.HasPrefix(ver, value) {
			if val, ok := lookAt[value]; ok {
				return corebase.ParseBase("win", val)
			}
		}
	}
	return corebase.Base{}, errors.Errorf("unknown series %q", ver)
}

func isWindowsNano() (bool, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, isNanoKey, registry.QUERY_VALUE)
	if err != nil {
		return false, errors.Capture(err)
	}
	defer k.Close()

	s, _, err := k.GetIntegerValue("NanoServer")
	if err != nil {
		return false, errors.Capture(err)
	}
	return s == 1, nil
}
