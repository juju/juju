// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package initsystems

func findInitExecutable() (string, error) {
	return "<windows>", nil
}
