// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"os"
	"strings"

	"github.com/gabriel-samfira/sys/windows/registry"
	"github.com/juju/errors"
)

var (
	// currentVersionKey is defined as a variable instead of a constant
	// to allow overwriting during testing
	currentVersionKey = "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion"

	// isNanoKey determines the registry key that can be queried to determine whether
	// a machine is a nano machine
	isNanoKey = "Software\\Microsoft\\Windows NT\\CurrentVersion\\Server\\ServerLevels"
)

func getVersionFromRegistry() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, currentVersionKey, registry.QUERY_VALUE)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer k.Close()
	s, _, err := k.GetStringValue("ProductName")
	if err != nil {
		return "", errors.Trace(err)
	}

	return s, nil
}

func readSeries() (string, error) {
	ver, err := getVersionFromRegistry()
	if err != nil {
		return "unknown", errors.Trace(err)
	}

	var lookAt = windowsVersions

	isNano, err := isWindowsNano()
	if err != nil && os.IsNotExist(err) {
		return "unknown", errors.Trace(err)
	}
	if isNano {
		lookAt = windowsNanoVersions
	}

	for _, value := range windowsVersionMatchOrder {
		if strings.HasPrefix(ver, value) {
			if val, ok := lookAt[value]; ok {
				return val, nil
			}
		}
	}
	return "unknown", errors.Errorf("unknown series %q", ver)
}

func isWindowsNano() (bool, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, isNanoKey, registry.QUERY_VALUE)
	if err != nil {
		return false, errors.Trace(err)
	}
	defer k.Close()

	s, _, err := k.GetIntegerValue("NanoServer")
	if err != nil {
		return false, errors.Trace(err)
	}
	return s == 1, nil
}
