// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//+build !linux

package lxcutils

func runningInsideLXC() (bool, error) {
	return false, nil
}
