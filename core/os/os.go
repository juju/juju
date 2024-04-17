// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"strings"
)

var HostOS = hostOS // for monkey patching

// HostOSTypeName returns the name of the host OS.
func HostOSTypeName() (osTypeName string) {
	defer func() {
		if err := recover(); err != nil {
			osTypeName = "unknown"
		}
	}()
	return strings.ToLower(HostOS().String())
}
