// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package base

import (
	"syscall"

	corebase "github.com/juju/juju/core/base"
)

var sysctlVersion = func() (string, error) {
	return syscall.Sysctl("kern.osrelease")
}

func readBase() (corebase.Base, error) {
	channel, err := sysctlVersion()
	if err != nil {
		return corebase.Base{}, err
	}
	return corebase.ParseBase("osx", channel)
}
