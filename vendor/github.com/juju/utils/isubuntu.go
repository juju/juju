// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"strings"
)

// IsUbuntu executes lxb_release to see if the host OS is Ubuntu.
func IsUbuntu() bool {
	out, err := RunCommand("lsb_release", "-i", "-s")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "Ubuntu"
}
