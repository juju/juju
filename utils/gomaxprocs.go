// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os"
	"runtime"
	"sync"
)

var mu = sync.Mutex{}
var gomaxprocs = runtime.GOMAXPROCS
var numcpu = runtime.NumCPU

// OverrideGOMAXPROCSFuncs allows you to override calling runtime.GOMAXPROCS
// and runtime.NumCPU. This is exposed so that tests can poke at this without
// actually changing the runtime behavior.
func OverrideGOMAXPROCSFuncs(newGOMAXPROCS func(int) int, newNumCPU func() int) (cleanup func()) {
	mu.Lock()
	defer mu.Unlock()
	origGOMAXPROCS := gomaxprocs
	logger.Debugf("setting GOMAXPROCS to %#v", newGOMAXPROCS)
	gomaxprocs = newGOMAXPROCS
	origNumCPU := numcpu
	numcpu = newNumCPU
	return func() {
		mu.Lock()
		defer mu.Unlock()
		gomaxprocs = origGOMAXPROCS
		numcpu = origNumCPU
	}
}

// UseMultipleCPUs is called when we have decided we want to set GOMAXPROCS to
// the current number of CPU cores. This will not override the environment
// variable if it is set.
func UseMultipleCPUs() {
	mu.Lock()
	defer mu.Unlock()
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
