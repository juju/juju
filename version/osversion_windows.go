// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"strings"
	"syscall"
	"unsafe"

	"github.com/juju/errors"
)

func readRegString(h syscall.Handle, key string) (value string, err error) {
	var typ uint32
	var buf uint32

	// Get size of registry key
	err = syscall.RegQueryValueEx(h, syscall.StringToUTF16Ptr(key), nil, &typ, nil, &buf)
	if err != nil {
		return value, err
	}

	n := make([]uint16, buf/2+1)
	err = syscall.RegQueryValueEx(h, syscall.StringToUTF16Ptr(key), nil, &typ, (*byte)(unsafe.Pointer(&n[0])), &buf)
	if err != nil {
		return value, err
	}
	return syscall.UTF16ToString(n[:]), err
}

func getVersionFromRegistry() (string, error) {
	var h syscall.Handle
	err := syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE,
		syscall.StringToUTF16Ptr("SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion"),
		0, syscall.KEY_READ, &h)
	if err != nil {
		return "", err
	}
	defer syscall.RegCloseKey(h)
	str, err := readRegString(h, "ProductName")
	if err != nil {
		return "", err
	}
	return str, nil
}

func osVersion() (string, error) {
	ver, err := getVersionFromRegistry()
	if err != nil {
		return "unknown", err
	}
	if val, ok := windowsVersions[ver]; ok {
		return val, nil
	}
	for key, value := range windowsVersions {
		if strings.HasPrefix(ver, key) {
			return value, nil
		}
	}
	return "unknown", errors.Errorf("unknown series %q", ver)
}
