// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package version

func osVersion() string {
	return getWinVersion()
}
