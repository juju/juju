// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	stdos "os"
	"path/filepath"

	corebase "github.com/juju/juju/core/base"
)

func readBase() (corebase.Base, error) {
	values, err := readHostOSRelease(osReleaseFile)
	if err != nil {
		return corebase.Base{}, err
	}
	return corebase.ParseBase(values["ID"], values["VERSION_ID"])
}

// snapHostOSReleasePath returns the path to a host os-release file staged
// into SNAP_COMMON by cloud-init, or empty if not running under snap / not staged.
func snapHostOSReleasePath() string {
	snapCommon := stdos.Getenv("SNAP_COMMON")
	if snapCommon == "" {
		return ""
	}
	return filepath.Join(snapCommon, "host-os-release")
}
