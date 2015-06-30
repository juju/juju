// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package testing

import (
	"os"
	"syscall"
)

// Signals is the list of operating system signals this suite will capture
var Signals = []os.Signal{
	os.Interrupt,
	syscall.SIGTERM,
	syscall.SIGQUIT,
}
