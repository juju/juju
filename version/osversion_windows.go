// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"strings"

	"github.com/gabriel-samfira/sys/windows/registry"
	"github.com/juju/errors"
)

// currentVersionKey is defined as a variable instead of a constant
// to allow overwriting during testing
var currentVersionKey = "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion"

func getVersionFromRegistry() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, currentVersionKey, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	s, _, err := k.GetStringValue("ProductName")
	if err != nil {
		return "", err
	}

	return s, nil
}

func osVersion() (string, error) {
	ver, err := getVersionFromRegistry()
	if err != nil {
		return "unknown", err
	}
	if val, ok := windowsVersions[ver]; ok {
		return val, nil
	}
	for _, value := range windowsVersionMatchOrder {
		if strings.HasPrefix(ver, value) {
			if val, ok := windowsVersions[value]; ok {
				return val, nil
			}
		}
	}
	return "unknown", errors.Errorf("unknown series %q", ver)
}
