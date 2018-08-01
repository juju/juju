// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// +build !windows,!darwin,!linux

package os

func hostOS() OSType {
	return Unknown
}
