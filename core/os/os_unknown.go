// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

//go:build !windows && !darwin && !linux

package os

import "github.com/juju/juju/core/os/ostype"

func hostOS() ostype.OSType {
	return ostype.Unknown
}
