// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os"
	"runtime"
)

var gomaxprocs = runtime.GOMAXPROCS
var numcpu = runtime.NumCPU

// UseMultipleCPUs is called when we have decided we want to set GOMAXPROCS to
// the current number of CPU cores. This will not override the environment
// variable if it is set.
func UseMultipleCPUs() {
	if envGOMAXPROCS := os.Getenv("GOMAXPROCS"); envGOMAXPROCS != "" {
		n := gomaxprocs(0)
		logger.Debugf("GOMAXPROCS already set in environment to %q, %d internally",
			envGOMAXPROCS, n)
		return
	}
	n := numcpu()
	logger.Debugf("setting GOMAXPROCS to %d", n)
	gomaxprocs(n)
}
