// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package base

import (
	corebase "github.com/juju/juju/core/base"
	coreos "github.com/juju/juju/core/os"
)

var (
	// osReleaseFile is the name of the file that is read in order to determine
	// the linux type release version.
	osReleaseFile = "/etc/os-release"
)

func readBase() (corebase.Base, error) {
	values, err := coreos.ReadOSRelease(osReleaseFile)
	if err != nil {
		return corebase.Base{}, err
	}
	return corebase.ParseBase(values["ID"], values["VERSION_ID"])
}
