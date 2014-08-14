// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package container

import "fmt"

func flockLock(fd int) (err error) {
	return fmt.Errorf("not implemented")
}

func flockUnlock(fd int) (err error) {
	return fmt.Errorf("not implemented")
}
