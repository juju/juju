// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	corebase "github.com/juju/juju/core/base"
)

func readBase() (corebase.Base, error) {
	values, err := ReadOSRelease(osReleaseFile)
	if err != nil {
		return corebase.Base{}, err
	}
	return corebase.ParseBase(values["ID"], values["VERSION_ID"])
}
