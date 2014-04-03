// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os"
	"runtime"
)

var gomaxprocs = runtime.GOMAXPROCS
var numCPU = runtime.NumCPU

// UseMultipleCPUs sets GOMAXPROCS to the number of CPU cores unless it has
// already been overridden by the GOMAXPROCS environment variable.
func UseMultipleCPUs() {
	if envGOMAXPROCS := os.Getenv("GOMAXPROCS"); envGOMAXPROCS != "" {
		n := gomaxprocs(0)
		logger.Debugf("GOMAXPROCS already set in environment to %q, %d internally",
			envGOMAXPROCS, n)
		return
	}
	n := numCPU()
	logger.Debugf("setting GOMAXPROCS to %d", n)
	gomaxprocs(n)
}
